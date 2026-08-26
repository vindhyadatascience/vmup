package tui

type menuAction int

const (
	actionLaunch menuAction = iota
	actionEdit
	actionInfo
	actionStartTunnels
	actionStopTunnels
	actionSSH
	actionTransfer
	actionTransferTunnel
	actionDestroy
	actionStopAll
	actionAttachDiskToVM
	actionDetachDiskFromVM
	actionQuit
)

type backToMenuMsg struct{}
