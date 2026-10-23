package pool

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
)

// DefaultRefreshSkew 是过期前提前续期的宽限期。
// 留 5 分钟余量避免请求落在过期临界点引发鉴权失败。
const DefaultRefreshSkew = 5 * time.Minute

// Refresher 用 refresh_token 换取新的 access_token 并更新账号对象。
type Refresher func(ctx context.Context, a *Account) error

// Selector 从候选账号中挑选当前可用账号。
// 采用账号级别的细粒度互斥锁同步续期状态，避免刷新风暴，返回调用方快照副本。
// 当某个账号设置了 CooldownUntil 时，Pick 会自动跳过并顺延到下一可用账号。
type Selector struct {
	mu         sync.Mutex
	cands      []*Account
	refresh    Refresher
	skew       time.Duration
	now        func() time.Time
	lastErrs   error
	activeName string

	accountMu sync.Mutex
	locks     map[string]*sync.Mutex
}

// NewSelector 构造选择器。refresh 为 nil 时不刷新，仅使用现存 token。
func NewSelector(cands []*Account, refresh Refresher) *Selector {
	return &Selector{
		cands:   cands,
		refresh: refresh,
		skew:    DefaultRefreshSkew,
		now:     time.Now,
		locks:   make(map[string]*sync.Mutex),
	}
}

func (s *Selector) accountLock(name string) *sync.Mutex {
	s.accountMu.Lock()
	defer s.accountMu.Unlock()
	if s.locks == nil {
		s.locks = make(map[string]*sync.Mutex)
	}
	l, ok := s.locks[name]
	if !ok {
		l = new(sync.Mutex)
		s.locks[name] = l
	}
	return l
}

// CandidateCount 返回候选账号数量，供网关决定 429 最多换几次号。
//
// 只数「有名字的」非空候选：空槽位不能真的拿来发请求。
func (s *Selector) CandidateCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	n := 0
	for _, a := range s.cands {
		if a != nil {
			n++
		}
	}
	return n
}

// Candidates 返回候选账号的快照副本，仅供状态展示。
func (s *Selector) Candidates() []*Account {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]*Account, 0, len(s.cands))
	for _, a := range s.cands {
		if a == nil {
			continue
		}
		l := s.accountLock(a.Name)
		l.Lock()
		cp := *a
		l.Unlock()
		out = append(out, &cp)
	}
	return out
}

// LastErrors 返回最近一次 Pick 失败的原因。
func (s *Selector) LastErrors() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.lastErrs
}

func (s *Selector) tryPick(ctx context.Context, a *Account, now time.Time) (*Account, error) {
	if a == nil {
		return nil, errors.New("账号为空")
	}
	if cooling(a, now) {
		return nil, fmt.Errorf("%s: 冷却至 %s", a.Name, FormatCooldown(a.CooldownUntil))
	}

	l := s.accountLock(a.Name)

	l.Lock()
	needsRefresh := a.AccessToken == "" || withinSkew(a, now, s.skew)
	l.Unlock()

	if needsRefresh {
		if s.refresh != nil && a.RefreshToken != "" {
			s.mu.Unlock()

			l.Lock()
			now = s.now()
			var err error
			if a.AccessToken == "" || withinSkew(a, now, s.skew) {
				err = s.refresh(ctx, a)
			}
			l.Unlock()

			s.mu.Lock()
			now = s.now()

			// 重新加锁后再次检查冷却状态（可能在刷新期间被配额检测协程打上冷却）
			if cooling(a, now) {
				return nil, fmt.Errorf("%s: 冷却至 %s", a.Name, FormatCooldown(a.CooldownUntil))
			}

			if err != nil {
				l.Lock()
				expired := a.AccessToken == "" || hardExpired(a, now)
				l.Unlock()
				if expired {
					return nil, fmt.Errorf("%s 刷新失败: %w", a.Name, err)
				}
			}
		} else {
			l.Lock()
			expired := a.AccessToken == "" || hardExpired(a, now)
			l.Unlock()
			if expired {
				return nil, fmt.Errorf("%s: 无可用 token 且无法刷新", a.Name)
			}
		}
	}

	l.Lock()
	if a.AccessToken == "" {
		l.Unlock()
		return nil, fmt.Errorf("%s: access_token 为空", a.Name)
	}
	cp := *a
	l.Unlock()

	return &cp, nil
}

// AllCoolingError 表示池里每个账号都在冷却，但只是暂时的。
//
// 和「池空」「token 全废」必须分开：那种情况等多久都没用，必须让用户
// 去处理；而全池冷却只要等到 Until 就能自然恢复。上层据此决定是
// 回 503 + Retry-After（客户端稍后重试），还是报错让人介入。
//
// Until 取所有账号里**最早**的解冻时刻 —— 我们要的是「最快什么时候
// 能恢复服务」，不是「最后一个什么时候恢复」。
type AllCoolingError struct {
	Until time.Time
	// Names 是处于冷却的账号名，用于日志展示。
	Names []string
}

func (e *AllCoolingError) Error() string {
	if e == nil {
		return "全部账号冷却中"
	}
	names := strings.Join(e.Names, ", ")
	if names != "" {
		return fmt.Sprintf("全部 %d 个账号冷却中（%s），最早 %s 恢复",
			len(e.Names), names, FormatCooldown(e.Until))
	}
	return fmt.Sprintf("全部账号冷却中，最早 %s 恢复", FormatCooldown(e.Until))
}

// RetryAfter 返回建议的重试等待时长（用于 HTTP Retry-After）。
// 已经过期（或 Until 为零）时返回 0，调用方不设该头。
func (e *AllCoolingError) RetryAfter(now time.Time) time.Duration {
	if e == nil || e.Until.IsZero() {
		return 0
	}
	if d := e.Until.Sub(now); d > 0 {
		return d
	}
	return 0
}

// Pick 挑选可用账号快照。具备活跃账号粘性（Active Account Affinity），
// 账号需要续期时在账号粒度进行并发同步，不阻塞全局锁。
func (s *Selector) Pick(ctx context.Context) (*Account, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := s.now()
	var errs []error
	// allCooling 记录「所有失败都是冷却」这一事实，并找出最早解冻时刻。
	// 只要出现一个非冷却的失败（token 废了、没 refresh_token），就说明
	// 光等没用，必须让用户介入。
	allCooling := true
	var earliest time.Time
	var coolingNames []string
	usable := 0

	note := func(a *Account, err error) {
		errs = append(errs, err)
		usable++
		if a == nil || !cooling(a, now) {
			allCooling = false
			return
		}
		coolingNames = append(coolingNames, a.Name)
		if earliest.IsZero() || a.CooldownUntil.Before(earliest) {
			earliest = a.CooldownUntil
		}
	}

	// 1. 活跃账号粘性：优先沿用当前健康的活跃账号，避免旧账号解冻时反抢导致频繁切号
	if s.activeName != "" {
		var active *Account
		for _, a := range s.cands {
			if a != nil && a.Name == s.activeName {
				active = a
				break
			}
		}
		if active != nil {
			cp, err := s.tryPick(ctx, active, now)
			if err == nil {
				s.lastErrs = nil
				return cp, nil
			}
			note(active, err)
		}
	}

	// 2. 活跃账号不可用（或初次选号）：按候选列表挑选首个可用健康账号
	for _, a := range s.cands {
		if a == nil || a.Name == s.activeName {
			continue
		}
		cp, err := s.tryPick(ctx, a, now)
		if err == nil {
			s.activeName = cp.Name
			s.lastErrs = nil
			return cp, nil
		}
		note(a, err)
	}

	s.lastErrs = errors.Join(errs...)
	if usable == 0 {
		return nil, errors.New("账号池为空（先用 ags add <name> 捕获一个）")
	}
	if allCooling && len(coolingNames) > 0 {
		return nil, &AllCoolingError{Until: earliest, Names: coolingNames}
	}
	return nil, fmt.Errorf("没有可用账号: %w", s.lastErrs)
}

// CooldownOf 返回账号当前的冷却截止时刻，账号不存在返回零值。只读，供日志展示。
func (s *Selector) CooldownOf(name string) time.Time {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, a := range s.cands {
		if a != nil && a.Name == name {
			return a.CooldownUntil
		}
	}
	return time.Time{}
}

// SetCooldown 设置账号冷却截止时刻。处于冷却期的账号在 Pick 时会被自动跳过。
// until 为零值或过去时间表示解除冷却。
//
// 调用方必须自己说明来源（通过 SetCooldownCause），否则一律视为 429 避让 ——
// 保守假设：宁可多等，也不要让额度轮询洗掉一个真实的限流避让。
func (s *Selector) SetCooldown(name string, until time.Time) {
	s.SetCooldownCause(name, until, Cooldown429)
}

// SetCooldownCause 设置冷却并记录来源。
func (s *Selector) SetCooldownCause(name string, until time.Time, cause CooldownCause) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, a := range s.cands {
		if a == nil || a.Name != name {
			continue
		}
		a.CooldownUntil = until
		if until.IsZero() {
			a.CooldownCause = CooldownNone
		} else {
			a.CooldownCause = cause
		}
		return
	}
}

// CooldownCauseOf 返回账号当前冷却的来源，无冷却返回 CooldownNone。
func (s *Selector) CooldownCauseOf(name string) CooldownCause {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, a := range s.cands {
		if a != nil && a.Name == name {
			if a.CooldownUntil.IsZero() {
				return CooldownNone
			}
			return a.CooldownCause
		}
	}
	return CooldownNone
}

// SetCooldownMax 仅当 until 晚于现有冷却时才更新，避免缩短 429 等路径打上的更长避让。
func (s *Selector) SetCooldownMax(name string, until time.Time) {
	s.SetCooldownMaxCause(name, until, CooldownQuota)
}

// SetCooldownMaxCause 是 SetCooldownMax 的带来源版本。
func (s *Selector) SetCooldownMaxCause(name string, until time.Time, cause CooldownCause) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, a := range s.cands {
		if a == nil || a.Name != name {
			continue
		}
		if until.After(a.CooldownUntil) {
			a.CooldownUntil = until
			a.CooldownCause = cause
		}
		return
	}
}

// ClearExpiredCooldown 仅在现有冷却已过期（或无）时清零；
// 仍在未来的冷却（如刚打上的 429 避让）予以保留，防止健康轮询误洗。
func (s *Selector) ClearExpiredCooldown(name string, now time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, a := range s.cands {
		if a == nil || a.Name != name {
			continue
		}
		if a.CooldownUntil.IsZero() || !now.Before(a.CooldownUntil) {
			a.CooldownUntil = time.Time{}
			a.CooldownCause = CooldownNone
		}
		return
	}
}

// ClearQuotaCooldown 清掉**额度来源**的冷却，即便它还没到期。
//
// 为什么需要它：周窗口耗尽会把账号冻结到 resetTime，那可能是一周后。
// 一旦额度轮询确认「其实还有额度」（上游提前补量、或当初误判），
// 再让账号干等一周就完全说不通了。
//
// 429 来源的冷却不受影响 —— 那条路径必须在测试里钉死。
func (s *Selector) ClearQuotaCooldown(name string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, a := range s.cands {
		if a == nil || a.Name != name {
			continue
		}
		if a.CooldownCause == CooldownQuota {
			a.CooldownUntil = time.Time{}
			a.CooldownCause = CooldownNone
		}
		return
	}
}

// SetQuotaState 记录账号的额度耗尽标记（内存状态）。
func (s *Selector) SetQuotaState(name string, exhausted bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, a := range s.cands {
		if a == nil || a.Name != name {
			continue
		}
		a.QuotaExhausted = exhausted
		return
	}
}

// FreshToken 返回指定账号的有效 access_token。若已过期或接近过期，会自动调用 refresh 续期。
// 无论账号当前是否处于冷却状态，均可刷新（用于后台额度探测）。
func (s *Selector) FreshToken(ctx context.Context, name string) (string, error) {
	s.mu.Lock()
	var target *Account
	for _, a := range s.cands {
		if a != nil && a.Name == name {
			target = a
			break
		}
	}
	s.mu.Unlock()
	if target == nil {
		return "", fmt.Errorf("账号 %q 不在候选池中", name)
	}

	l := s.accountLock(target.Name)
	l.Lock()
	defer l.Unlock()

	now := s.now()
	needsRefresh := target.AccessToken == "" || withinSkew(target, now, s.skew)
	if needsRefresh && s.refresh != nil && target.RefreshToken != "" {
		if err := s.refresh(ctx, target); err != nil {
			if target.AccessToken == "" || hardExpired(target, now) {
				return "", fmt.Errorf("%s 刷新失败: %w", target.Name, err)
			}
		}
	}

	if target.AccessToken == "" {
		return "", fmt.Errorf("%s: access_token 为空", target.Name)
	}
	return target.AccessToken, nil
}

// RefreshAll 强制刷新全部候选账号，常用于启动预热。
// 并发刷新各账号（每账号持其细粒度锁与 Pick/FreshToken 互斥），不持全局锁，
// 避免单个账号的网络往返阻塞选号与额度轮询。
func (s *Selector) RefreshAll(ctx context.Context) error {
	if s.refresh == nil {
		return errors.New("未配置 Refresher，无法刷新")
	}

	s.mu.Lock()
	cands := append([]*Account(nil), s.cands...)
	s.mu.Unlock()

	var mu sync.Mutex
	var errs []error
	tried := 0
	var wg sync.WaitGroup
	for _, a := range cands {
		if a == nil {
			continue
		}
		wg.Add(1)
		go func(ac *Account) {
			defer wg.Done()
			l := s.accountLock(ac.Name)
			l.Lock()
			defer l.Unlock()
			if ac.RefreshToken == "" {
				return
			}
			mu.Lock()
			tried++
			mu.Unlock()
			if err := s.refresh(ctx, ac); err != nil {
				mu.Lock()
				errs = append(errs, fmt.Errorf("%s: %w", ac.Name, err))
				mu.Unlock()
			}
		}(a)
	}
	wg.Wait()

	mu.Lock()
	defer mu.Unlock()
	if tried == 0 {
		return errors.New("候选账号都没有 refresh_token，无法刷新")
	}
	return errors.Join(errs...)
}

// FormatCooldown 将冷却截止时刻格式化为易读时间文本。
func FormatCooldown(t time.Time) string {
	if t.IsZero() {
		return "-"
	}
	l := t.Local()
	now := time.Now()
	if l.Year() == now.Year() && l.YearDay() == now.YearDay() {
		return l.Format("15:04:05")
	}
	return l.Format("01-02 15:04")
}

// cooling 判断账号当前是否处于冷却期。
func cooling(a *Account, now time.Time) bool {
	return !a.CooldownUntil.IsZero() && now.Before(a.CooldownUntil)
}

// withinSkew 判断是否已进入「过期前 skew 就该刷新」的窗口。
// Expiry 为零值表示不知道过期时间，此时不主动刷新（避免每次请求都刷）。
func withinSkew(a *Account, now time.Time, skew time.Duration) bool {
	if a.Expiry.IsZero() {
		return false
	}
	return !now.Before(a.Expiry.Add(-skew))
}

// hardExpired 判断 token 是否已真正过期（而非仅进入宽限期）。
func hardExpired(a *Account, now time.Time) bool {
	return !a.Expiry.IsZero() && now.After(a.Expiry)
}
