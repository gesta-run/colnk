package protocol

const (
	MajorVersion                = 4
	MinorVersion                = 1
	MaxHeaderBytes              = 64 << 10
	MaxPayloadBytes             = 1 << 20
	MaxResponsePayloadBytes     = 8 << 20
	DirectoryPagePayloadBytes   = 1 << 20
	KindFile                    = "file"
	KindTCP                     = "tcp"
	KindDNS                     = "dns"
	HandshakeErrorUnauthorized  = "unauthorized"
	HandshakeErrorSessionActive = "session_active"
)

type Handshake struct {
	MajorVersion int           `json:"major_version"`
	MinorVersion int           `json:"minor_version"`
	APIKey       string        `json:"api_key"`
	Policy       NetworkPolicy `json:"network_policy,omitempty"`
}

type HandshakeAck struct {
	Accepted     bool          `json:"accepted"`
	ErrorCode    string        `json:"error_code,omitempty"`
	Error        string        `json:"error,omitempty"`
	MajorVersion int           `json:"major_version,omitempty"`
	MinorVersion int           `json:"minor_version,omitempty"`
	Policy       NetworkPolicy `json:"network_policy,omitempty"`
}

type NetworkPolicy struct {
	AllowedCIDRs      []string `json:"allowed_cidrs,omitempty"`
	AllowedPorts      []uint16 `json:"allowed_ports,omitempty"`
	DNSSuffixes       []string `json:"dns_suffixes,omitempty"`
	MaxTCPConnections int      `json:"max_tcp_connections,omitempty"`
}

func DefaultNetworkPolicy() NetworkPolicy {
	return NetworkPolicy{
		AllowedCIDRs: []string{"100.64.0.1/32"},
		DNSSuffixes:  []string{"colnk"},
	}
}

type Request struct {
	Kind          string `json:"kind"`
	Operation     string `json:"operation,omitempty"`
	Path          string `json:"path,omitempty"`
	NewPath       string `json:"new_path,omitempty"`
	Target        string `json:"target,omitempty"`
	Offset        int64  `json:"offset,omitempty"`
	Size          int    `json:"size,omitempty"`
	Mode          uint32 `json:"mode,omitempty"`
	ModTimeNS     int64  `json:"mod_time_ns,omitempty"`
	SetModTime    bool   `json:"set_mod_time,omitempty"`
	Sync          bool   `json:"sync,omitempty"`
	DataLength    int    `json:"data_length,omitempty"`
	DataEncoding  string `json:"data_encoding,omitempty"`
	RawDataLength int    `json:"raw_data_length,omitempty"`
}

type Response struct {
	ErrorCode     int       `json:"error_code,omitempty"`
	Error         string    `json:"error,omitempty"`
	Attr          *FileAttr `json:"attr,omitempty"`
	More          bool      `json:"more,omitempty"`
	DataLength    int       `json:"data_length,omitempty"`
	DataEncoding  string    `json:"data_encoding,omitempty"`
	RawDataLength int       `json:"raw_data_length,omitempty"`
}

type FileAttr struct {
	Mode      uint32 `json:"mode"`
	Size      uint64 `json:"size"`
	ModTimeNS int64  `json:"mod_time_ns"`
	Inode     uint64 `json:"inode,omitempty"`
	UID       uint32 `json:"uid,omitempty"`
	GID       uint32 `json:"gid,omitempty"`
}

type DirEntry struct {
	Name       string   `json:"name"`
	Attr       FileAttr `json:"attr"`
	LinkTarget string   `json:"link_target,omitempty"`
}
