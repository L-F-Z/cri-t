package types

// ContainerInfo stores information about containers.
type ContainerInfo struct {
	Name            string            `json:"name"`
	Pid             int               `json:"pid"`
	Image           string            `json:"image"`     // If set, _some_ name of the image imageID; it may have NO RELATIONSHIP to the users’ requested image name.
	ImageRef        string            `json:"image_ref"` // In the format of StorageImageID.StringForOutOfProcessConsumptionOnly(), or "".
	CreatedTime     int64             `json:"created_time"`
	Labels          map[string]string `json:"labels"`
	Annotations     map[string]string `json:"annotations"`
	CrioAnnotations map[string]string `json:"crio_annotations"`
	LogPath         string            `json:"log_path"`
	Root            string            `json:"root"`
	Sandbox         string            `json:"sandbox"`
	IPs             []string          `json:"ip_addresses"`
}

// CrioInfo stores information about the crio daemon.
type CrioInfo struct {
	StorageRoot  string `json:"storage_root"`
	CgroupDriver string `json:"cgroup_driver"`
}
