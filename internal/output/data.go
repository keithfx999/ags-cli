package output

// Canonical Data types for JSON envelope. All fields are PascalCase.
// Commands construct these instead of map[string]any.

type InstanceData struct {
	Id             string `json:"Id"`
	ToolId         string `json:"ToolId"`
	ToolName       string `json:"ToolName"`
	Status         string `json:"Status"`
	CreatedAt      string `json:"CreatedAt"`
	UpdatedAt      string `json:"UpdatedAt,omitempty"`
	TimeoutSeconds *uint64 `json:"TimeoutSeconds,omitempty"`
	ExpiresAt      string `json:"ExpiresAt,omitempty"`
	StopReason     string `json:"StopReason,omitempty"`
	Endpoints      any    `json:"Endpoints,omitempty"`
	MountOptions   any    `json:"MountOptions,omitempty"`
}

type InstanceListData struct {
	Items      []InstanceData  `json:"Items"`
	Pagination *PaginationData `json:"Pagination"`
}

type PaginationData struct {
	Offset     int  `json:"Offset"`
	Limit      int  `json:"Limit"`
	Total      int  `json:"Total"`
	NextCursor *string `json:"NextCursor"`
}

type CodeRunData struct {
	Stdout         string `json:"Stdout"`
	Stderr         string `json:"Stderr"`
	Results        any    `json:"Results"`
	Error          any    `json:"Error"`
	ExecutionCount int    `json:"ExecutionCount"`
}

type ExecData struct {
	Stdout   string `json:"Stdout"`
	Stderr   string `json:"Stderr"`
	ExitCode int    `json:"ExitCode"`
}

type FileOpData struct {
	Operation string `json:"Operation"`
	Path      string `json:"Path"`
	LocalPath string `json:"LocalPath,omitempty"`
	Size      int64  `json:"Size,omitempty"`
}

type BrowserData struct {
	InstanceId string `json:"InstanceId"`
	VncUrl     string `json:"VncUrl"`
	CdpUrl     string `json:"CdpUrl"`
}

type DeleteData struct {
	Deleted   int      `json:"Deleted"`
	Failed    int      `json:"Failed"`
	FailedIds []string `json:"FailedIds,omitempty"`
}

type VersionData struct {
	Version   string `json:"Version"`
	Commit    string `json:"Commit"`
	BuildTime string `json:"BuildTime"`
}
