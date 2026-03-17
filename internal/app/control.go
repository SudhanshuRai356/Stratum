package app

type ControlCommandType string

const (
	CmdPause      ControlCommandType = "pause"
	CmdResume     ControlCommandType = "resume"
	CmdReannounce ControlCommandType = "reannounce"
	CmdStop       ControlCommandType = "stop"
)

type ControlCommand struct {
	Type ControlCommandType
}
