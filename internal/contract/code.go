package contract

import "time"

// Code view shapes for /api/sessions/{id}/code/*.

type CodeContext struct {
	ID     string `json:"contextId"`
	Root   string `json:"rootPath"`
	Kind   string `json:"rootKind"`
	Browse bool   `json:"browseAvailable"`
	Diff   bool   `json:"diffAvailable"`
	Ignore bool   `json:"ignoreAvailable"`
	Reason string `json:"reason"`
	Write  bool   `json:"writableAvailable"`
}

type CodeNode struct {
	ID            string `json:"nodeId"`
	Path          string `json:"path"`
	Name          string `json:"name"`
	Kind          string `json:"kind"`
	Status        string `json:"gitStatus"`
	Ignored       bool   `json:"ignored"`
	Referenceable bool   `json:"referenceable"`
	Link          string `json:"linkTarget,omitempty"`
	MediaType     string `json:"mediaType"`
}

// CodeListing is a directory page or a search result.
type CodeListing struct {
	Children []CodeNode `json:"children"`
	Next     string     `json:"nextCursor"`
	Limited  bool       `json:"limited"`
	Complete bool       `json:"complete"`
	Version  string     `json:"contentVersion"`
	Checked  time.Time  `json:"checkedAt"`
}

// CodeLocation is a root-relative node found from a typed path.
type CodeLocation struct {
	NodeID string `json:"nodeId"`
	Path   string `json:"path"`
	Kind   string `json:"kind"`
}

type CodeDocument struct {
	NodeID        string `json:"nodeId"`
	Path          string `json:"path"`
	Absolute      string `json:"absolutePath"`
	Source        string `json:"source"`
	OID           string `json:"oid"`
	Version       string `json:"contentVersion"`
	Kind          string `json:"kind"`
	Text          string `json:"text"`
	Lines         int    `json:"lineCount"`
	Referenceable bool   `json:"referenceable"`
	Reason        string `json:"reason"`
	MediaType     string `json:"mediaType"`
	Size          int64  `json:"size"`
	Downloadable  bool   `json:"downloadable"`
}

type CodeFile struct {
	Snapshot string       `json:"snapshotId"`
	Document CodeDocument `json:"document"`
	Checked  time.Time    `json:"checkedAt"`
}

type CodeChange struct {
	NodeID        string `json:"nodeId"`
	ID            string `json:"changeId"`
	Group         string `json:"group"`
	OldPath       string `json:"oldPath"`
	NewPath       string `json:"newPath"`
	Status        string `json:"status"`
	OldOID        string `json:"oldOid"`
	NewOID        string `json:"newOid"`
	OldMode       string `json:"oldMode"`
	NewMode       string `json:"newMode"`
	ReviewVersion string `json:"reviewVersion,omitempty"`
}

type CodeChanges struct {
	Next    string       `json:"nextCursor"`
	Total   int          `json:"total"`
	Entries []CodeChange `json:"entries"`
	Version string       `json:"contentVersion"`
	Checked time.Time    `json:"checkedAt"`
}

type CodeDiff struct {
	Snapshot string       `json:"snapshotId"`
	Old      CodeDocument `json:"old"`
	New      CodeDocument `json:"new"`
	Kind     string       `json:"kind"`
	Reason   string       `json:"reason"`
	Checked  time.Time    `json:"checkedAt"`
	Change   CodeChange   `json:"change"`
}

// CodeCheckRequest is the body of POST .../code/check.
type CodeCheckRequest struct {
	Context        string            `json:"contextId"`
	Snapshots      []string          `json:"snapshotIds"`
	Directories    map[string]string `json:"directories"`
	Changes        string            `json:"changesVersion"`
	IncludeIgnored bool              `json:"includeIgnored"`
}

// CodeCheck reports which snapshots, directories and change lists are stale.
type CodeCheck struct {
	Snapshots   map[string]string `json:"snapshots"`
	Directories map[string]string `json:"directories"`
	Changes     string            `json:"changes"`
	Checked     time.Time         `json:"checkedAt"`
}

// CodeRenameRequest is the body of POST .../code/rename.
type CodeRenameRequest struct {
	Context string `json:"contextId"`
	NodeID  string `json:"nodeId"`
	Name    string `json:"name"`
}

// CodeMkdirRequest is the body of POST .../code/mkdir.
type CodeMkdirRequest struct {
	Context string `json:"contextId"`
	Parent  string `json:"parentNodeId"`
	Name    string `json:"name"`
}

// CodeCopyRequest is the body of POST .../code/copy; Mode is "copy" or "move".
type CodeCopyRequest struct {
	Context string   `json:"contextId"`
	NodeIDs []string `json:"nodeIds"`
	Target  string   `json:"targetNodeId"`
	Mode    string   `json:"mode"`
}

// CodeMutation lists the nodes a file operation produced, under final names.
type CodeMutation struct {
	Nodes []CodeNode `json:"nodes"`
}

// CodeUsage counts the entries and file bytes at and below one node.
type CodeUsage struct {
	Entries int   `json:"entries"`
	Bytes   int64 `json:"bytes"`
	Limited bool  `json:"limited"`
}
