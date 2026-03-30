package tui

type screen int

const (
	screenMain screen = iota // tabbed main view (Instances / Data Disks)
	screenConfig
	screenProgress
	screenStatus
	screenConfirmDestroy
	screenConfirmDestroyName
	screenConfirmStopVM
	screenConfirmStopAll
	screenDiskCreate
	screenDiskImport
	screenDiskConfirmDelete
	screenDiskConfirmDeleteName
	screenDiskResize
	screenDiskAttach
	screenDiskAttachConfirm
	screenDiskMountOptions
	screenDiskDetach
	screenDiskDetachFromVM
)

type tab int

const (
	tabInstances tab = iota
	tabDataDisks
)
