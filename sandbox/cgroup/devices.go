// -*- Mode: Go; indent-tabs-mode: t -*-

package cgroup

// SnapDeviceCgroupOptions is a stub - device cgroup tracking not supported.
type SnapDeviceCgroupOptions struct {
	SelfManaged bool
	NonStrict   bool
}

// SnapDeviceFile is a stub - device cgroup tracking not supported.
func SnapDeviceFile(securityTag string) string {
	return ""
}

// LoadSnapDeviceCgroupOptions is a stub.
func LoadSnapDeviceCgroupOptions(securityTag string) (SnapDeviceCgroupOptions, error) {
	return SnapDeviceCgroupOptions{}, nil
}

func (opts *SnapDeviceCgroupOptions) MarshalText() ([]byte, error) {
	return nil, nil
}
