package main

import (
	"context"
	"log"
	"math/rand"
	"time"

	"github.com/kylesean/ags/internal/pool"
	"github.com/kylesean/ags/internal/quota"
)

// quotaWatcher 定期查池中每个账号的 GEMINI 额度，耗尽就打冷却、恢复就解冻。
//
// 为什么放在后台轮询，而不是每次选号时现查：
// 查一次额度是一次网络往返，挂在请求路径上会让每个请求都慢一拍，
// 而额度一分钟才变一次。冷却一旦写进 Selector，
// 下一次 Pick 就自然跳过那个号 —— 换号逻辑不需要碰转发路径一行代码。
//
// 判据是只读的 retrieveUserQuotaSummary，不消耗生成额度，
// 所以「拿现有没额度的账号反复测」是安全的。
type quotaWatcher struct {
	sel       *pool.Selector
	upstream  string
	userAgent string
	threshold float64
	log       *log.Logger

	// last 记录各账号上一次的耗尽状态，用于去重日志
	last map[string]bool
	// trigger 承载 429 当事账号名（cap 1，只留最新），空串表示全量。
	trigger chan string
}

func newQuotaWatcher(sel *pool.Selector, upstream, userAgent string,
	threshold float64, lg *log.Logger) *quotaWatcher {
	return &quotaWatcher{
		sel:       sel,
		upstream:  upstream,
		userAgent: userAgent,
		threshold: threshold,
		log:       lg,
		last:      make(map[string]bool),
		trigger:   make(chan string, 1),
	}
}

// jitteredInterval 给轮询间隔叠加 ±25% 抖动，打散机械周期节拍。
// base <= 0 时返回 0（调用方以 0 表示关闭）。
func jitteredInterval(base time.Duration) time.Duration {
	if base <= 0 {
		return 0
	}
	half := int64(base) / 4
	d := rand.Int63n(2*half+1) - half
	return base + time.Duration(d)
}

// recoveryPollFloor 是全池冷却时的轮询下限。
//
// 为什么不设成 0：那会退化成热循环，几分钟内把额度接口刷爆，
// 反而可能被上游限流。30 秒足够「恢复后很快察觉」，又不至于过分。
const recoveryPollFloor = 30 * time.Second

// nextQuotaInterval 决定下一轮额度轮询的等待时间。
//
// 常规情况按 base（默认 3 分钟）走。但如果整个池都在冷却，说明
// 现在这个状态是**暂时**的，恢复时刻就是翻盘点：这时按 base 等
// 最多要白等 3 分钟，期间的请求全部撞 503。压到 floor 能显著缩短
// 这段空窗。
//
// 冷却很久（周窗口耗尽、要等一周）时不做这个压缩：那种情况下
// 30 秒一次的探测纯属浪费，按常规节奏走即可。
func nextQuotaInterval(sel *pool.Selector, base time.Duration, now time.Time) time.Duration {
	if base <= 0 || sel == nil {
		return base
	}

	earliest := time.Time{}
	allCooling := true
	n := 0
	for _, a := range sel.Candidates() {
		if a == nil {
			continue
		}
		n++
		if a.CooldownUntil.IsZero() || !now.Before(a.CooldownUntil) {
			allCooling = false
			break
		}
		if earliest.IsZero() || a.CooldownUntil.Before(earliest) {
			earliest = a.CooldownUntil
		}
	}
	if n == 0 || !allCooling || earliest.IsZero() {
		return base
	}

	// 只在「恢复时刻近到能被 floor 覆盖」时才压缩。
	// 留 resetGrace 的余量：上游落账有延迟，早到一点点照样 429。
	remaining := earliest.Sub(now) + time.Minute
	if remaining <= 0 || remaining > base {
		return base
	}
	if remaining < recoveryPollFloor {
		return recoveryPollFloor
	}
	return remaining
}

// triggerAccount 异步触发单号即时额度检测（非阻塞，只留最新）。
// 429 当事号只查自己，把突发流量从 N 次降到 1 次。
func (q *quotaWatcher) triggerAccount(name string) {
	select {
	case <-q.trigger:
	default:
	}
	select {
	case q.trigger <- name:
	default:
	}
}

// check 跑一轮额度检测。first 为真时无论状态如何都打一行 ——
// 启动那一轮要看到每个账号的全量额度，而不是只看到翻转。
//
// 单个账号查失败只告警并保持原状态：额度接口抖一下不该把号锁死，
// 而 cooldown 本来就有 resetTime 兜底，下一轮会纠正。
func (q *quotaWatcher) check(ctx context.Context, first bool) {
	now := time.Now()
	for _, a := range q.sel.Candidates() {
		q.probe(ctx, a, now, first)
	}
}

// checkOne 只检测指定账号（429 单号即查），找不到时静默跳过。
func (q *quotaWatcher) checkOne(ctx context.Context, name string) {
	now := time.Now()
	for _, a := range q.sel.Candidates() {
		if a.Name == name {
			q.probe(ctx, a, now, false)
			return
		}
	}
}

func (q *quotaWatcher) probe(ctx context.Context, a *pool.Account, now time.Time, first bool) {
	tok, err := q.sel.FreshToken(ctx, a.Name)
	if err != nil {
		q.log.Printf("查额度 %s 获取 token 失败（保持原状态）: %v", a.Name, err)
		return
	}
	ck, cancel := context.WithTimeout(ctx, 15*time.Second)
	sum, err := quota.Fetch(ck, q.upstream, tok, q.userAgent)
	cancel()
	if err != nil {
		q.log.Printf("查额度 %s 失败（保持原状态）: %v", a.Name, err)
		return
	}

	g := sum.Gemini()
	until, exhausted := g.Exhausted(q.threshold, now)
	changed := first || q.last[a.Name] != exhausted
	q.last[a.Name] = exhausted
	q.sel.SetQuotaState(a.Name, exhausted)

	if exhausted {
		q.sel.SetCooldownMaxCause(a.Name, until, pool.CooldownQuota)
		if changed {
			q.log.Printf("额度耗尽 %s: %s → 冷却至 %s",
				a.Name, g.Describe(), pool.FormatCooldown(until))
		}
		return
	}
	// 额度确认可用：额度来源的冷却（可能远在 resetTime）立刻解除，
	// 但 429 避让保持不动。
	q.sel.ClearQuotaCooldown(a.Name)
	q.sel.ClearExpiredCooldown(a.Name, now)
	if changed {
		q.log.Printf("额度 %s: %s", a.Name, g.Describe())
	}
}

// run 按 interval 循环 check（每轮叠加抖动），ctx 取消即退。首轮由调用方同步执行。
//
// 间隔每轮重新计算（nextQuotaInterval）：全池冷却时缩短，恢复后自动回到常规节奏。
func (q *quotaWatcher) run(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		return
	}
	for {
		delay := nextQuotaInterval(q.sel, interval, time.Now())
		select {
		case <-ctx.Done():
			return
		case name := <-q.trigger:
			if name == "" {
				q.check(ctx, false)
			} else {
				q.checkOne(ctx, name)
			}
		case <-time.After(jitteredInterval(delay)):
			q.check(ctx, false)
		}
	}
}
