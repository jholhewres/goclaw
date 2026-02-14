package webui

// AssistantAdapter wraps a generic set of callbacks to satisfy the AssistantAPI
// interface. This avoids a direct import cycle between copilot and webui.
type AssistantAdapter struct {
	GetConfigMapFn       func() map[string]any
	ListSessionsFn       func() []SessionInfo
	GetSessionMessagesFn func(sessionID string) []MessageInfo
	GetUsageGlobalFn     func() UsageInfo
	GetChannelHealthFn   func() []ChannelHealthInfo
	GetSchedulerJobsFn   func() []JobInfo
	ListSkillsFn         func() []SkillInfo
	SendChatMessageFn    func(sessionID, content string) (string, error)
}

func (a *AssistantAdapter) GetConfigMap() map[string]any {
	if a.GetConfigMapFn != nil {
		return a.GetConfigMapFn()
	}
	return nil
}

func (a *AssistantAdapter) ListSessions() []SessionInfo {
	if a.ListSessionsFn != nil {
		return a.ListSessionsFn()
	}
	return nil
}

func (a *AssistantAdapter) GetSessionMessages(sessionID string) []MessageInfo {
	if a.GetSessionMessagesFn != nil {
		return a.GetSessionMessagesFn(sessionID)
	}
	return nil
}

func (a *AssistantAdapter) GetUsageGlobal() UsageInfo {
	if a.GetUsageGlobalFn != nil {
		return a.GetUsageGlobalFn()
	}
	return UsageInfo{}
}

func (a *AssistantAdapter) GetChannelHealth() []ChannelHealthInfo {
	if a.GetChannelHealthFn != nil {
		return a.GetChannelHealthFn()
	}
	return nil
}

func (a *AssistantAdapter) GetSchedulerJobs() []JobInfo {
	if a.GetSchedulerJobsFn != nil {
		return a.GetSchedulerJobsFn()
	}
	return nil
}

func (a *AssistantAdapter) ListSkills() []SkillInfo {
	if a.ListSkillsFn != nil {
		return a.ListSkillsFn()
	}
	return nil
}

func (a *AssistantAdapter) SendChatMessage(sessionID, content string) (string, error) {
	if a.SendChatMessageFn != nil {
		return a.SendChatMessageFn(sessionID, content)
	}
	return "", nil
}
