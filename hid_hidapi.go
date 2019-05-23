// +build freebsd darwin

package hid

import "C"

/*
#cgo freebsd CFLAGS: -I/usr/local/include/hidapi
#cgo freebsd LDFLAGS: -L/usr/local/lib -Wl,-Bstatic -lhidapi -liconv -Wl,-Bdynamic -lusb

#include <stdlib.h>
#include <hidapi.h>
*/
import "C"

import (
	"errors"
	"fmt"
	"sync"
	"unsafe"
)

type hidapiDevice struct {
	info   *DeviceInfo
	device *C.hid_device

	readSetup sync.Once
	readErr   error
	readCh    chan []byte
}

func Devices() ([]*DeviceInfo, error) {
	device_list := C.hid_enumerate(0, 0)
	if device_list == nil {
		return nil, errors.New("unknown error enumerating usb devices")
	}
	defer C.hid_free_enumeration(device_list)

	var devices []*DeviceInfo
	device := device_list
	for device != nil {
		info, err := getDeviceInfo(device)
		if err != nil {
			return nil, err
		}
		devices = append(devices, info)
		device = device.next
	}

	return devices, nil
}

func getDeviceInfo(device *C.struct_hid_device_info) (*DeviceInfo, error) {
	manufacturer, err := wcharTToString(device.manufacturer_string)
	if err != nil {
		return nil, fmt.Errorf("unable to convert manufacturer string: %v", err)
	}
	product, err := wcharTToString(device.product_string)
	if err != nil {
		return nil, fmt.Errorf("unable to convert product string: %v", err)
	}

	return &DeviceInfo{
		Path:          C.GoString(device.path),
		VendorID:      uint16(device.vendor_id),
		ProductID:     uint16(device.product_id),
		VersionNumber: uint16(device.release_number),
		Manufacturer:  manufacturer,
		Product:       product,
		UsagePage:     uint16(device.usage_page),
		Usage:         uint16(device.usage),

		InputReportLength:  64, // Default size of raw HID report
		OutputReportLength: 64,
	}, nil
}

func ByPath(path string) (*DeviceInfo, error) {
	devices, err := Devices()
	if err != nil {
		return nil, err
	}
	for _, deviceInfo := range devices {
		if deviceInfo.Path == path {
			return deviceInfo, nil
		}
	}
	return nil, nil
}

func (d *DeviceInfo) Open() (Device, error) {
	p := C.CString(d.Path)
	defer C.free(unsafe.Pointer(p))
	device := C.hid_open_path(p)
	if device == nil {
		return nil, fmt.Errorf("unable to open device: %s", d.Path)
	}

	return &hidapiDevice{info: d, device: device}, nil
}

func (d *hidapiDevice) Close() {
	C.hid_close(d.device)
}

func (d *hidapiDevice) Write(data []byte) error {
	dat := (*C.uchar)(C.CBytes(data))
	defer C.free(unsafe.Pointer(dat))
	n := C.hid_write(d.device, dat, C.ulong(len(data)))
	if int(n) != len(data) {
		return fmt.Errorf("only wrote %d bytes", int(n))
	}
	return nil
}

func (d *hidapiDevice) ReadCh() <-chan []byte {
	d.readSetup.Do(func() {
		d.readCh = make(chan []byte, 30)
		go d.readThread()
	})
	return d.readCh
}

func (d *hidapiDevice) ReadError() error {
	return d.readErr
}

func (d *hidapiDevice) readThread() {
	defer close(d.readCh)
	bufLen := C.ulong(d.info.InputReportLength)
	buf := (*C.uchar)(C.malloc(bufLen))
	defer C.free(unsafe.Pointer(buf))
	for {
		n := C.hid_read(d.device, buf, bufLen)
		if n == -1 {
			d.readErr = errors.New("unable to read from HID device")
			return
		}
		d.readCh <- C.GoBytes(unsafe.Pointer(buf), n)
	}
}
