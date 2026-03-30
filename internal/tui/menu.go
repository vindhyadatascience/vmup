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
	actionAttachDiskToVM
	actionDetachDiskFromVM
	actionQuit
)

type backToMenuMsg struct{}
