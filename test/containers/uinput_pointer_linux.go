//go:build ignore

// Command uinput-pointer creates a pointer device that never sends an event,
// prints where the kernel put it, and holds it open until it is killed.
//
// A compositor gives its seat a pointer only when it has a pointer device to
// give it, and some dialogs — a password prompt that grabs the seat — will not
// take input from a seat that has none. This device exists to be counted, not
// to be used: every click a test performs goes through the compositor's own
// cursor, so nothing here ever emits an event, and a device that emits nothing
// cannot reach a session outside this container.
//
// The node is printed because a container has no udev to create it: whoever
// starts this is expected to mknod it from the numbers below.
//
// Built by the container image rather than by the module — it is a tool of the
// test environment, not part of SSHakku.
package main

import (
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"
	"unsafe"
)

// The uinput ioctls and event codes this needs, from linux/uinput.h and
// linux/input-event-codes.h. They are spelled out rather than pulled from a
// dependency: five constants are cheaper to read than another module.
const (
	uiSetEvBit  = 0x40045564
	uiSetKeyBit = 0x40045565
	uiSetRelBit = 0x40045566
	uiDevCreate = 0x5501

	evKey = 0x01
	evRel = 0x02

	btnLeft  = 0x110
	btnRight = 0x111
	relX     = 0x00
	relY     = 0x01

	deviceName = "sshakku-test-pointer"
)

// uinputUserDev is the struct the kernel expects written to /dev/uinput before
// UI_DEV_CREATE: a name padded to 80 bytes, the bus/vendor/product/version
// quadruple, and the absolute-axis arrays a pointer does not use.
type uinputUserDev struct {
	Name       [80]byte
	ID         struct{ BusType, Vendor, Product, Version uint16 }
	EffectsMax uint32
	AbsMax     [64]int32
	AbsMin     [64]int32
	AbsFuzz    [64]int32
	AbsFlat    [64]int32
}

func main() {
	fd, err := syscall.Open("/dev/uinput", syscall.O_WRONLY|syscall.O_NONBLOCK, 0)
	if err != nil {
		fail("open /dev/uinput: %v", err)
	}

	for _, call := range []struct {
		request uintptr
		value   uintptr
	}{
		{uiSetEvBit, evKey},
		{uiSetEvBit, evRel},
		{uiSetKeyBit, btnLeft},
		{uiSetKeyBit, btnRight},
		{uiSetRelBit, relX},
		{uiSetRelBit, relY},
	} {
		if err := ioctl(fd, call.request, call.value); err != nil {
			fail("ioctl %#x %#x: %v", call.request, call.value, err)
		}
	}

	var dev uinputUserDev
	copy(dev.Name[:], deviceName)
	dev.ID.BusType, dev.ID.Vendor, dev.ID.Product, dev.ID.Version = 0x03, 1, 1, 1
	if _, err := syscall.Write(fd, (*[unsafe.Sizeof(dev)]byte)(unsafe.Pointer(&dev))[:]); err != nil {
		fail("write device description: %v", err)
	}
	if err := ioctl(fd, uiDevCreate, 0); err != nil {
		fail("create the device: %v", err)
	}

	node, devnum, err := findNode()
	if err != nil {
		fail("%v", err)
	}
	fmt.Printf("node: %s devnum: %s\n", node, devnum)
	os.Stdout.Sync()

	// Holding the descriptor open is what keeps the device alive; closing it
	// destroys it, which is also how a test tears it down: kill this process.
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop
}

// findNode returns the event node the kernel gave this device, and its
// major:minor, by looking for the name in sysfs. The device appears there a
// moment after it is created, so this waits for it rather than assuming.
func findNode() (string, string, error) {
	deadline := time.Now().Add(5 * time.Second)
	for {
		entries, err := filepath.Glob("/sys/class/input/event*")
		if err != nil {
			return "", "", fmt.Errorf("look for the device in sysfs: %w", err)
		}
		for _, entry := range entries {
			name, err := os.ReadFile(filepath.Join(entry, "device", "name"))
			if err != nil || strings.TrimSpace(string(name)) != deviceName {
				continue
			}
			devnum, err := os.ReadFile(filepath.Join(entry, "dev"))
			if err != nil {
				return "", "", fmt.Errorf("read the device number: %w", err)
			}
			return filepath.Base(entry), strings.TrimSpace(string(devnum)), nil
		}
		if time.Now().After(deadline) {
			return "", "", fmt.Errorf("the device did not appear in sysfs as %q", deviceName)
		}
		time.Sleep(100 * time.Millisecond)
	}
}

func ioctl(fd int, request, value uintptr) error {
	if _, _, errno := syscall.Syscall(syscall.SYS_IOCTL, uintptr(fd), request, value); errno != 0 {
		return errno
	}
	return nil
}

func fail(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "uinput-pointer: "+format+"\n", args...)
	os.Exit(1)
}
