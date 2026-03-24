package tui

type menuAction int

const (
	actionLaunch menuAction = iota
	actionEdit
	actionInfo
	actionStartTunnels
	actionStopTunnels
	actionSSH
	actionDestroy
	actionStopAll
	actionQuit
)

type backToMenuMsg struct{}
