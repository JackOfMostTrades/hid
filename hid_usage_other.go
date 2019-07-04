// +build !linux

package hid

func getDeviceUsageInfo(info *DeviceInfo) error {
	info.Usage = 0
	info.UsagePage = 0
	return nil
}
