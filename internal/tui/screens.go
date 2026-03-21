package tui

type screen int

const (
	screenVMList screen = iota
	screenConfig
	screenProgress
	screenStatus
	screenConfirmDestroy
	screenConfirmStopVM
	screenConfirmStopAll
)
