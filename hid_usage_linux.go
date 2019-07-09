package hid

// #include <linux/hidraw.h>
import "C"

import (
	"os"
	"syscall"
	"unsafe"
)

const (
	typeBits      = 8
	numberBits    = 8
	sizeBits      = 14
	directionRead = 2

	numberShift    = 0
	typeShift      = numberShift + numberBits
	sizeShift      = typeShift + typeBits
	directionShift = sizeShift + sizeBits
)

func ioc(dir, t, nr, size uintptr) uintptr {
	return (dir << directionShift) | (t << typeShift) | (nr << numberShift) | (size << sizeShift)
}

// ioR used for an ioctl that reads data from the device driver. The driver will be allowed to return sizeof(data_type) bytes to the user.
func ioR(t, nr, size uintptr) uintptr {
	return ioc(directionRead, t, nr, size)
}

// ioctl simplified ioct call
func ioctl(fd, op, arg uintptr) error {
	_, _, ep := syscall.Syscall(syscall.SYS_IOCTL, fd, op, arg)
	if ep != 0 {
		return syscall.Errno(ep)
	}
	return nil
}

var (
	ioctlHIDIOCGRDESCSIZE = ioR('H', 0x01, C.sizeof_int)
	ioctlHIDIOCGRDESC     = ioR('H', 0x02, C.sizeof_struct_hidraw_report_descriptor)
	ioctlHIDIOCGRAWINFO   = ioR('H', 0x03, C.sizeof_struct_hidraw_devinfo)
)

func getDeviceUsageInfo(info *DeviceInfo) error {
	dev, err := os.OpenFile(info.Path, os.O_RDWR, 0)
	if err != nil {
		if os.IsPermission(err) {
			info.Usage = 0
			info.UsagePage = 0
			return nil
		}
		return err
	}
	defer dev.Close()
	fd := uintptr(dev.Fd())

	var descSize C.int
	if err := ioctl(fd, ioctlHIDIOCGRDESCSIZE, uintptr(unsafe.Pointer(&descSize))); err != nil {
		return err
	}

	rawDescriptor := C.struct_hidraw_report_descriptor{
		size: C.__u32(descSize),
	}
	if err := ioctl(fd, ioctlHIDIOCGRDESC, uintptr(unsafe.Pointer(&rawDescriptor))); err != nil {
		return err
	}
	usagePage, usage := parseReportDescriptor(C.GoBytes(unsafe.Pointer(&rawDescriptor.value), descSize))
	info.UsagePage = usagePage
	info.Usage = usage
	return nil
}

// very basic report parser that will pull out the usage page, usage, and the
// sizes of the first input and output reports
func parseReportDescriptor(b []byte) (usagePage uint16, usage uint16) {
	for len(b) > 0 {
		// read item size, type, and tag
		size := int(b[0] & 0x03)
		if size == 3 {
			size = 4
		}
		typ := (b[0] >> 2) & 0x03
		tag := (b[0] >> 4) & 0x0f
		b = b[1:]

		if len(b) < size {
			return
		}

		// read item value
		var v uint64
		for i := 0; i < size; i++ {
			v += uint64(b[i]) << (8 * uint(i))
		}
		b = b[size:]

		switch {
		case typ == 1 && tag == 0 && usagePage == 0: // usage page
			usagePage = uint16(v)
		case typ == 2 && tag == 0 && usage == 0: // usage
			usage = uint16(v)
		}

		if usagePage > 0 && usage > 0 {
			return
		}
	}
	return
}
