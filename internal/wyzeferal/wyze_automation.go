package wyzeferal

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// ──────────────────────────────────────────────
// Types
// ──────────────────────────────────────────────

type AutomationType string

const (
	TypeScheduled AutomationType = "scheduled" // simple multi-device list at a scheduled time
	TypeTimer     AutomationType = "timer"     // device on for X minutes then auto-off
	TypeSafety    AutomationType = "safety"    // turn off device if left on longer than X min
	TypeScene     AutomationType = "scene"     // multi-device workflow with per-step delays (stagger)
)

// RunMode controls whether an automation only runs when you tap Run, or also on a schedule.
type RunMode string

const (
	ModeManual    RunMode = "manual"    // run only via POST / run button (no calendar)
	ModeScheduled RunMode = "scheduled" // fires at Schedule; may also run manually
)

// ScheduleConfig defines when a scheduled automation runs.
type ScheduleConfig struct {
	// DaysOfWeek: 0=Sun, 1=Mon … 6=Sat. Empty means every day.
	DaysOfWeek []int  `json:"days_of_week"`
	TimeHHMM   string `json:"time"`   // "22:30" local to Timezone (or server local if empty)
	Action     string `json:"action"` // "on" | "off" | "toggle" — used for TypeScheduled only

	// Advanced scheduling (common smart-home style; more can be added over time).
	Timezone     string `json:"timezone,omitempty"`       // IANA, e.g. "America/Los_Angeles"
	EndsOnDate   string `json:"ends_on_date,omitempty"`     // "2006-01-02" optional; stops after this calendar day in Timezone
	SkipWeekends bool   `json:"skip_weekends,omitempty"`    // skip Sat/Sun
}

// SceneStep is one device action inside a Scene automation.
type SceneStep struct {
	Order         int    `json:"order"`
	DeviceMAC     string `json:"device_mac"`
	DeviceName    string `json:"device_name"`
	DeviceModel   string `json:"device_model"`
	Action        string `json:"action"`           // "on" | "off" | "toggle" | "color" | "brightness"
	Color         string `json:"color,omitempty"`  // "#rrggbb" — required when Action == "color"
	Brightness    int    `json:"brightness,omitempty"` // 1-100 — required when Action == "brightness"
	DelayBefore   int    `json:"delay_before_sec"` // seconds to wait before executing this step
	SkipIfAlready bool   `json:"skip_if_already"`  // skip if device is already in target state
}

// Automation is the root struct stored in JSON.
type Automation struct {
	ID          string         `json:"id"`
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	Type        AutomationType `json:"type"`
	Enabled     bool           `json:"enabled"`

	// RunMode: manual = only explicit runs; scheduled = also fires on Schedule (requires Schedule set).
	RunMode RunMode `json:"run_mode,omitempty"`

	// StaggerBetweenDevicesSec: wait this many seconds between each device for TypeScheduled
	// and TypeTimer (OFF phase). Ignored for TypeScene (use SceneSteps[].delay_before_sec).
	StaggerBetweenDevicesSec int `json:"stagger_between_devices_sec,omitempty"`

	// Used by TypeScheduled
	Schedule     *ScheduleConfig `json:"schedule,omitempty"`
	DeviceMACs   []string        `json:"device_macs,omitempty"`   // for scheduled/timer/safety
	DeviceModels []string        `json:"device_models,omitempty"` // parallel to DeviceMACs
	DeviceNames  []string        `json:"device_names,omitempty"`  // parallel to DeviceMACs

	// Used by TypeTimer – on for N minutes then off
	OnForMinutes int `json:"on_for_minutes,omitempty"`

	// Used by TypeSafety – turn off if device left on for N minutes
	SafetyMaxOnMinutes int `json:"safety_max_on_minutes,omitempty"`

	// Used by TypeScene
	SceneSteps []SceneStep `json:"scene_steps,omitempty"`

	// Status
	LastRunAt     *time.Time `json:"last_run_at,omitempty"`
	LastRunResult string     `json:"last_run_result,omitempty"` // "ok" | error message
	IsRunning     bool       `json:"is_running"`
	NextRunAt     *time.Time `json:"next_run_at,omitempty"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// ──────────────────────────────────────────────
// AutomationStore – persistence
// ──────────────────────────────────────────────

type AutomationStore struct {
	mu   sync.RWMutex
	path string
	data []Automation
}

func NewAutomationStore(path string) *AutomationStore {
	s := &AutomationStore{path: path}
	_ = s.load()
	return s
}

func (s *AutomationStore) load() error {
	raw, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return json.Unmarshal(raw, &s.data)
}

func (s *AutomationStore) save() error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(s.data, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.path, raw, 0o600)
}

func (s *AutomationStore) List() []Automation {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]Automation, len(s.data))
	copy(out, s.data)
	return out
}

func (s *AutomationStore) Get(id string) (Automation, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, a := range s.data {
		if a.ID == id {
			return a, true
		}
	}
	return Automation{}, false
}

func (s *AutomationStore) Upsert(a Automation) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, existing := range s.data {
		if existing.ID == a.ID {
			s.data[i] = a
			return s.save()
		}
	}
	s.data = append(s.data, a)
	return s.save()
}

func (s *AutomationStore) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, a := range s.data {
		if a.ID == id {
			s.data = append(s.data[:i], s.data[i+1:]...)
			return s.save()
		}
	}
	return fmt.Errorf("automation %q not found", id)
}

func (s *AutomationStore) UpdateStatus(id string, isRunning bool, result string, ranAt time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, a := range s.data {
		if a.ID == id {
			s.data[i].IsRunning = isRunning
			if result != "" {
				s.data[i].LastRunResult = result
				t := ranAt
				s.data[i].LastRunAt = &t
			}
			_ = s.save()
			return
		}
	}
}

func (s *AutomationStore) SetNextRun(id string, t *time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, a := range s.data {
		if a.ID == id {
			s.data[i].NextRunAt = t
			_ = s.save()
			return
		}
	}
}

// InferRunMode returns stored run_mode or infers from schedule (legacy JSON without run_mode).
func InferRunMode(a Automation) RunMode {
	if a.RunMode != "" {
		return a.RunMode
	}
	if a.Schedule != nil {
		return ModeScheduled
	}
	return ModeManual
}

// ──────────────────────────────────────────────
// AutomationLogger
// ──────────────────────────────────────────────

type AutomationLogger struct {
	mu   sync.Mutex
	path string
}

func NewAutomationLogger(path string) *AutomationLogger {
	_ = os.MkdirAll(filepath.Dir(path), 0o755)
	return &AutomationLogger{path: path}
}

func (l *AutomationLogger) Log(level, format string, args ...any) {
	l.mu.Lock()
	defer l.mu.Unlock()

	msg := fmt.Sprintf(format, args...)
	line := fmt.Sprintf("%s [%-5s] %s\n",
		time.Now().Format("2006-01-02 15:04:05"),
		level,
		msg,
	)

	f, err := os.OpenFile(l.path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		fmt.Printf("[AutomationLogger] cannot open log: %v\n", err)
		return
	}
	defer f.Close()
	_, _ = f.WriteString(line)
	fmt.Print(line)
}

func (l *AutomationLogger) Info(format string, args ...any)  { l.Log("INFO ", format, args...) }
func (l *AutomationLogger) Warn(format string, args ...any)  { l.Log("WARN ", format, args...) }
func (l *AutomationLogger) Error(format string, args ...any) { l.Log("ERROR", format, args...) }
func (l *AutomationLogger) Step(format string, args ...any)  { l.Log("STEP", format, args...) }

// Tail returns the last n lines of the log file.
func (l *AutomationLogger) Tail(n int) []string {
	l.mu.Lock()
	defer l.mu.Unlock()

	raw, err := os.ReadFile(l.path)
	if err != nil {
		return nil
	}
	lines := strings.Split(strings.TrimRight(string(raw), "\n"), "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return lines
}

// ──────────────────────────────────────────────
// AutomationScheduler
// ──────────────────────────────────────────────

// AutomationScheduler manages background scheduling of automations.
type AutomationScheduler struct {
	store     *AutomationStore
	logger    *AutomationLogger
	getClient func() *WyzeClient // lazy; reads latest config

	stopCh chan struct{}
	wg     sync.WaitGroup
	once   sync.Once
}

func NewAutomationScheduler(store *AutomationStore, logger *AutomationLogger, getClient func() *WyzeClient) *AutomationScheduler {
	return &AutomationScheduler{
		store:     store,
		logger:    logger,
		getClient: getClient,
		stopCh:    make(chan struct{}),
	}
}

// Start launches the background scheduler loop.
func (sch *AutomationScheduler) Start() {
	sch.once.Do(func() {
		sch.wg.Add(1)
		go sch.loop()
		sch.logger.Info("Scheduler started")
	})
}

// Stop halts the scheduler and waits for running automations to finish.
func (sch *AutomationScheduler) Stop() {
	close(sch.stopCh)
	sch.wg.Wait()
	sch.logger.Info("Scheduler stopped")
}

func (sch *AutomationScheduler) loop() {
	defer sch.wg.Done()

	// Compute next-run times for all scheduled automations on boot.
	sch.refreshNextRunTimes()

	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-sch.stopCh:
			return
		case now := <-ticker.C:
			sch.tick(now)
		}
	}
}

func (sch *AutomationScheduler) tick(now time.Time) {
	automations := sch.store.List()
	for _, a := range automations {
		if !a.Enabled || a.IsRunning {
			continue
		}
		if InferRunMode(a) != ModeScheduled || a.Schedule == nil {
			continue
		}
		switch a.Type {
		case TypeScheduled, TypeScene:
			if sch.shouldRunScheduled(a, now) {
				sch.triggerAsync(a)
			}
		default:
			// Timer / safety: manual run only unless we add schedule later
		}
	}
}

func (sch *AutomationScheduler) scheduleLocation(a Automation) *time.Location {
	if a.Schedule != nil && strings.TrimSpace(a.Schedule.Timezone) != "" {
		if loc, err := time.LoadLocation(strings.TrimSpace(a.Schedule.Timezone)); err == nil {
			return loc
		}
	}
	return time.Local
}

// shouldRunScheduled returns true if the automation's schedule matches 'now' (within ~30s of target).
func (sch *AutomationScheduler) shouldRunScheduled(a Automation, now time.Time) bool {
	if a.Schedule == nil {
		return false
	}

	loc := sch.scheduleLocation(a)
	nowZ := now.In(loc)

	if strings.TrimSpace(a.Schedule.EndsOnDate) != "" {
		end, err := time.ParseInLocation("2006-01-02", strings.TrimSpace(a.Schedule.EndsOnDate), loc)
		if err == nil {
			today := time.Date(nowZ.Year(), nowZ.Month(), nowZ.Day(), 0, 0, 0, 0, loc)
			if today.After(end) {
				return false
			}
		}
	}

	if a.Schedule.SkipWeekends {
		wd := int(nowZ.Weekday())
		if wd == 0 || wd == 6 {
			return false
		}
	}

	// Check day of week.
	if len(a.Schedule.DaysOfWeek) > 0 {
		weekday := int(nowZ.Weekday())
		matched := false
		for _, d := range a.Schedule.DaysOfWeek {
			if d == weekday {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}

	parts := strings.SplitN(a.Schedule.TimeHHMM, ":", 2)
	if len(parts) != 2 {
		return false
	}
	var hh, mm int
	if _, err := fmt.Sscanf(parts[0], "%d", &hh); err != nil {
		return false
	}
	if _, err := fmt.Sscanf(parts[1], "%d", &mm); err != nil {
		return false
	}

	target := time.Date(nowZ.Year(), nowZ.Month(), nowZ.Day(), hh, mm, 0, 0, loc)
	diff := nowZ.Sub(target)
	if diff < 0 {
		diff = -diff
	}

	if diff > 30*time.Second {
		return false
	}

	if a.LastRunAt != nil && now.Sub(*a.LastRunAt) < 5*time.Minute {
		return false
	}

	return true
}

func (sch *AutomationScheduler) refreshNextRunTimes() {
	automations := sch.store.List()
	now := time.Now()
	for _, a := range automations {
		if InferRunMode(a) != ModeScheduled || a.Schedule == nil {
			continue
		}
		if a.Type != TypeScheduled && a.Type != TypeScene {
			continue
		}
		next := sch.nextRunTime(a, now)
		sch.store.SetNextRun(a.ID, next)
	}
}

// RefreshNextRunTimes recomputes NextRunAt for all scheduled automations (call after edits).
func (sch *AutomationScheduler) RefreshNextRunTimes() {
	sch.refreshNextRunTimes()
}

func (sch *AutomationScheduler) nextRunTime(a Automation, from time.Time) *time.Time {
	if a.Schedule == nil {
		return nil
	}
	parts := strings.SplitN(a.Schedule.TimeHHMM, ":", 2)
	if len(parts) != 2 {
		return nil
	}
	var hh, mm int
	if _, err := fmt.Sscanf(parts[0], "%d", &hh); err != nil {
		return nil
	}
	if _, err := fmt.Sscanf(parts[1], "%d", &mm); err != nil {
		return nil
	}

	loc := sch.scheduleLocation(a)
	fromZ := from.In(loc)

	candidate := time.Date(fromZ.Year(), fromZ.Month(), fromZ.Day(), hh, mm, 0, 0, loc)
	if candidate.Before(fromZ) {
		candidate = candidate.Add(24 * time.Hour)
	}

	skipWeekend := func(t time.Time) bool {
		if !a.Schedule.SkipWeekends {
			return false
		}
		wd := int(t.Weekday())
		return wd == 0 || wd == 6
	}

	// Advance past invalid days (wrong weekday filter or skip weekends).
	for i := 0; i < 14; i++ {
		if skipWeekend(candidate) {
			candidate = candidate.Add(24 * time.Hour)
			continue
		}
		if len(a.Schedule.DaysOfWeek) > 0 {
			weekday := int(candidate.Weekday())
			matched := false
			for _, d := range a.Schedule.DaysOfWeek {
				if d == weekday {
					matched = true
					break
				}
			}
			if !matched {
				candidate = candidate.Add(24 * time.Hour)
				continue
			}
		}
		if strings.TrimSpace(a.Schedule.EndsOnDate) != "" {
			end, err := time.ParseInLocation("2006-01-02", strings.TrimSpace(a.Schedule.EndsOnDate), loc)
			if err == nil {
				day := time.Date(candidate.Year(), candidate.Month(), candidate.Day(), 0, 0, 0, 0, loc)
				if day.After(end) {
					return nil
				}
			}
		}
		break
	}
	return &candidate
}

// triggerAsync runs an automation in the background.
func (sch *AutomationScheduler) triggerAsync(a Automation) {
	sch.wg.Add(1)
	go func() {
		defer sch.wg.Done()
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
		defer cancel()
		sch.RunAutomation(ctx, a.ID)
	}()
}

// RunAutomation executes an automation by ID, blocking until complete.
func (sch *AutomationScheduler) RunAutomation(ctx context.Context, id string) error {
	a, ok := sch.store.Get(id)
	if !ok {
		return fmt.Errorf("automation %q not found", id)
	}

	sch.logger.Info("Starting automation name=%q type=%s", a.Name, a.Type)
	sch.store.UpdateStatus(id, true, "", time.Time{})
	startedAt := time.Now()

	var runErr error
	switch a.Type {
	case TypeScheduled:
		runErr = sch.runScheduled(ctx, a)
	case TypeTimer:
		runErr = sch.runTimer(ctx, a)
	case TypeSafety:
		runErr = sch.runSafety(ctx, a)
	case TypeScene:
		runErr = sch.runScene(ctx, a)
	default:
		runErr = fmt.Errorf("unknown automation type: %s", a.Type)
	}

	result := "ok"
	if runErr != nil {
		result = runErr.Error()
		sch.logger.Error("Automation %q failed: %v", a.Name, runErr)
	} else {
		sch.logger.Info("Automation %q completed successfully", a.Name)
	}

	sch.store.UpdateStatus(id, false, result, startedAt)

	if InferRunMode(a) == ModeScheduled && a.Schedule != nil && (a.Type == TypeScheduled || a.Type == TypeScene) {
		next := sch.nextRunTime(a, time.Now())
		sch.store.SetNextRun(id, next)
	}

	return runErr
}

// ──────────────────────────────────────────────
// Runner implementations
// ──────────────────────────────────────────────

func (sch *AutomationScheduler) runScheduled(ctx context.Context, a Automation) error {
	if a.Schedule == nil {
		return fmt.Errorf("scheduled automation missing schedule config")
	}
	client := sch.getClient()
	action := a.Schedule.Action

	for i, mac := range a.DeviceMACs {
		if i > 0 && a.StaggerBetweenDevicesSec > 0 {
			sch.logger.Step("Stagger: waiting %ds before next device", a.StaggerBetweenDevicesSec)
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(time.Duration(a.StaggerBetweenDevicesSec) * time.Second):
			}
		}
		model := ""
		name := mac
		if i < len(a.DeviceModels) {
			model = a.DeviceModels[i]
		}
		if i < len(a.DeviceNames) {
			name = a.DeviceNames[i]
		}
		if err := sch.applyAction(ctx, client, mac, model, name, action); err != nil {
			sch.logger.Error("Device %q action=%s error: %v", name, action, err)
		}
	}
	return nil
}

func (sch *AutomationScheduler) runTimer(ctx context.Context, a Automation) error {
	if a.OnForMinutes <= 0 {
		return fmt.Errorf("timer automation needs on_for_minutes > 0")
	}
	client := sch.getClient()

	// Turn all devices on first (optional stagger).
	for i, mac := range a.DeviceMACs {
		if i > 0 && a.StaggerBetweenDevicesSec > 0 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(time.Duration(a.StaggerBetweenDevicesSec) * time.Second):
			}
		}
		model, name := modelAndName(a, i)
		sch.logger.Step("Timer: turning ON device=%q", name)
		if err := sch.applyAction(ctx, client, mac, model, name, "on"); err != nil {
			sch.logger.Error("Timer ON failed for %q: %v", name, err)
		}
	}

	// Wait for the configured duration.
	sch.logger.Info("Timer: waiting %d minutes before turning off", a.OnForMinutes)
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(time.Duration(a.OnForMinutes) * time.Minute):
	}

	// Turn all devices off (optional stagger — e.g. bed routine).
	for i, mac := range a.DeviceMACs {
		if i > 0 && a.StaggerBetweenDevicesSec > 0 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(time.Duration(a.StaggerBetweenDevicesSec) * time.Second):
			}
		}
		model, name := modelAndName(a, i)
		sch.logger.Step("Timer: turning OFF device=%q after %dm", name, a.OnForMinutes)
		if err := sch.applyAction(ctx, client, mac, model, name, "off"); err != nil {
			sch.logger.Error("Timer OFF failed for %q: %v", name, err)
		}
	}
	return nil
}

func (sch *AutomationScheduler) runSafety(ctx context.Context, a Automation) error {
	if a.SafetyMaxOnMinutes <= 0 {
		return fmt.Errorf("safety automation needs safety_max_on_minutes > 0")
	}
	client := sch.getClient()
	if !client.IsConfigured() {
		return fmt.Errorf("wyze client not configured")
	}

	for i, mac := range a.DeviceMACs {
		if i > 0 && a.StaggerBetweenDevicesSec > 0 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(time.Duration(a.StaggerBetweenDevicesSec) * time.Second):
			}
		}
		model, name := modelAndName(a, i)
		sch.logger.Step("Safety: checking state for device=%q", name)

		isOn, err := client.GetDeviceProperty(ctx, mac, model)
		if err != nil {
			sch.logger.Error("Safety: cannot read state for %q: %v", name, err)
			continue
		}
		if !isOn {
			sch.logger.Info("Safety: device=%q is already OFF, nothing to do", name)
			continue
		}

		sch.logger.Info("Safety: device=%q is ON, turning off (max=%dm)", name, a.SafetyMaxOnMinutes)
		if err := client.ControlDevice(ctx, mac, model, false); err != nil {
			sch.logger.Error("Safety: failed to turn off %q: %v", name, err)
		} else {
			sch.logger.Info("Safety: device=%q turned OFF", name)
		}
	}
	return nil
}

func (sch *AutomationScheduler) runScene(ctx context.Context, a Automation) error {
	client := sch.getClient()
	sch.logger.Info("Scene %q: %d steps", a.Name, len(a.SceneSteps))

	for idx, step := range a.SceneSteps {
		// Wait for the configured pre-step delay.
		if step.DelayBefore > 0 {
			sch.logger.Step("Scene step %d: waiting %ds before %s on %q",
				idx+1, step.DelayBefore, step.Action, step.DeviceName)
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(time.Duration(step.DelayBefore) * time.Second):
			}
		}

		// Check current state if SkipIfAlready is set.
		if step.SkipIfAlready && client.IsConfigured() {
			isOn, err := client.GetDeviceProperty(ctx, step.DeviceMAC, step.DeviceModel)
			if err != nil {
				sch.logger.Error("Scene step %d: cannot read state for %q: %v", idx+1, step.DeviceName, err)
			} else {
				targetOn := step.Action == "on"
				if targetOn == isOn {
					sch.logger.Step("Scene step %d: %q already %s – skipping",
						idx+1, step.DeviceName, step.Action)
					continue
				}
			}
		}

		sch.logger.Step("Scene step %d: %s %q", idx+1, step.Action, step.DeviceName)
		var stepErr error
		switch step.Action {
		case "color":
			clr := step.Color
			if clr == "" {
				clr = "#ffffff"
			}
			stepErr = client.SetColor(ctx, step.DeviceMAC, step.DeviceModel, clr)
		case "brightness":
			bri := step.Brightness
			if bri < 1 {
				bri = 1
			}
			if bri > 100 {
				bri = 100
			}
			stepErr = client.SetBrightness(ctx, step.DeviceMAC, step.DeviceModel, bri)
		default:
			stepErr = sch.applyAction(ctx, client, step.DeviceMAC, step.DeviceModel, step.DeviceName, step.Action)
		}
		if stepErr != nil {
			sch.logger.Error("Scene step %d failed for %q: %v", idx+1, step.DeviceName, stepErr)
			// Continue with the rest of the steps.
		} else {
			sch.logger.Step("Scene step %d: %q → %s ✓", idx+1, step.DeviceName, step.Action)
		}
	}

	return nil
}

// applyAction sends on/off/toggle to a device.
func (sch *AutomationScheduler) applyAction(ctx context.Context, client *WyzeClient, mac, model, name, action string) error {
	if !client.IsConfigured() {
		return fmt.Errorf("wyze client not configured (add API credentials in Settings)")
	}

	var on bool
	switch action {
	case "on":
		on = true
	case "off":
		on = false
	case "toggle":
		current, err := client.GetDeviceProperty(ctx, mac, model)
		if err != nil {
			return fmt.Errorf("cannot read current state for toggle: %w", err)
		}
		on = !current
		sch.logger.Step("Toggle %q: current=%v → %v", name, current, on)
	default:
		return fmt.Errorf("unknown action %q", action)
	}

	return client.ControlDevice(ctx, mac, model, on)
}

func modelAndName(a Automation, i int) (model, name string) {
	if i < len(a.DeviceMACs) {
		name = a.DeviceMACs[i]
	}
	if i < len(a.DeviceModels) {
		model = a.DeviceModels[i]
	}
	if i < len(a.DeviceNames) {
		name = a.DeviceNames[i]
	}
	return
}

// newID generates a simple time-based unique ID.
func newID() string {
	return fmt.Sprintf("%d", time.Now().UnixNano())
}
