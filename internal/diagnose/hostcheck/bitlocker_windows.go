//go:build windows

package hostcheck

import (
	"errors"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

// The shell is asked what BitLocker is doing rather than BitLocker itself.
//
// The documented way to this answer is the Win32_EncryptableVolume WMI class,
// and it is refused to anyone who is not an administrator — which is almost
// everybody who runs `sshakku doctor`, so taking that route would answer "could
// not tell" to the very people the check exists for. The shell answers the same
// question to an ordinary account, having asked on its behalf.
//
// What Microsoft documents about this property is its type and not its values,
// so the mapping below is deliberately one-sided: see bitLockerProtection.

const (
	// protectionPropertyName is the shell's name for what BitLocker is doing
	// with a volume. It is resolved to a property key by name rather than
	// written here as a GUID, so nothing in this file depends on a number
	// nobody can check by reading it.
	protectionPropertyName = "System.Volume.BitLockerProtection"

	// protectionOn is the one value read as a volume whose key is out of
	// reach. Every other value this build knows of describes a volume that is
	// unencrypted, cannot be encrypted, or is somewhere partway through being
	// encrypted or decrypted — none of which is a protected volume.
	protectionOn int32 = 1
	// protectionHighestKnown is the largest value with a known meaning.
	protectionHighestKnown int32 = 6
)

// shellItem2IID is IID_IShellItem2. A wrong one here is not a silent mistake:
// SHCreateItemFromParsingName refuses an interface the object does not
// implement, so the read fails and the report says it could not tell.
var shellItem2IID = windows.GUID{
	Data1: 0x7e9fb0d3,
	Data2: 0x919f,
	Data3: 0x4307,
	Data4: [8]byte{0xab, 0x2e, 0x9b, 0x18, 0x60, 0x31, 0x0c, 0x93},
}

var (
	shell32                         = windows.NewLazySystemDLL("shell32.dll")
	procSHCreateItemFromParsingName = shell32.NewProc("SHCreateItemFromParsingName")

	propsys                      = windows.NewLazySystemDLL("propsys.dll")
	procPSGetPropertyKeyFromName = propsys.NewProc("PSGetPropertyKeyFromName")
)

// errNoShellAnswer is the read not having worked — a volume the shell would not
// speak about, an interface it would not hand over, a property it does not
// carry. It is one error because the report does the same thing with all of
// them: says it could not tell.
var errNoShellAnswer = errors.New("the shell did not answer for this volume")

// propertyKey mirrors PROPERTYKEY field for field and in order: a format GUID
// and an index within it. The system reads these bytes directly, so a field
// added, removed or reordered here is not a compile error anywhere — it is a
// property key pointing somewhere else.
type propertyKey struct {
	FmtID windows.GUID
	PID   uint32
}

// iShellItem2Vtbl mirrors the IShellItem2 vtable in declaration order,
// inherited methods first. Only GetInt32 is called; the rest are here because
// a slot's position is what identifies it, so one left out would silently move
// every method after it.
type iShellItem2Vtbl struct {
	QueryInterface                   uintptr
	AddRef                           uintptr
	Release                          uintptr
	BindToHandler                    uintptr
	GetParent                        uintptr
	GetDisplayName                   uintptr
	GetAttributes                    uintptr
	Compare                          uintptr
	GetPropertyStore                 uintptr
	GetPropertyStoreWithCreateObject uintptr
	GetPropertyStoreForKeys          uintptr
	GetPropertyDescriptionList       uintptr
	Update                           uintptr
	GetProperty                      uintptr
	GetCLSID                         uintptr
	GetFileTime                      uintptr
	GetInt32                         uintptr
	GetString                        uintptr
	GetUInt32                        uintptr
	GetUInt64                        uintptr
	GetBool                          uintptr
}

type iShellItem2 struct {
	vtbl *iShellItem2Vtbl
}

func (i *iShellItem2) release() {
	_, _, _ = syscall.SyscallN(i.vtbl.Release, uintptr(unsafe.Pointer(i)))
}

func (i *iShellItem2) getInt32(key *propertyKey) (int32, error) {
	var value int32
	hr, _, _ := syscall.SyscallN(i.vtbl.GetInt32,
		uintptr(unsafe.Pointer(i)),
		uintptr(unsafe.Pointer(key)),
		uintptr(unsafe.Pointer(&value)))
	if hr != 0 {
		return 0, errNoShellAnswer
	}
	return value, nil
}

// bitLockerProtection turns what the shell says about a volume into the
// report's three answers.
//
// One value is read as protected and every other known value as not, which is
// not symmetry missing but the direction the mistake has to fall in. Microsoft
// documents this property's type without documenting its values, so a value
// that changes meaning, or one this build has never seen, must never arrive at
// "your keys are on an encrypted disk" — a reader who believes that and is
// wrong stops looking. Understating protection sends somebody to check a
// setting that turns out to be on already, which costs them a minute.
//
// Partway counts as not protected on purpose, and it is the line Microsoft
// draws for the status it does document: a partially encrypted volume is
// PROTECTION OFF, because the key is still readable on the disk.
func bitLockerProtection(value int32) *bool {
	if value < 0 || value > protectionHighestKnown {
		// A number with no meaning in this build. Answering either way would be
		// inventing one.
		return nil
	}
	protected := value == protectionOn
	return &protected
}

// volumeRootOf returns the root of the lettered volume a path is on, and
// whether there was one.
//
// A path with no drive letter — a UNC share, a volume GUID path, anything
// relative — is not refused because it could not be handled, but because the
// question does not apply to it: whether "the disk" is encrypted is not
// something this can answer about somebody else's file server.
func volumeRootOf(target string) (string, bool) {
	volume := filepath.VolumeName(target)
	if len(volume) != 2 || volume[1] != ':' {
		return "", false
	}
	letter := volume[0]
	if !strings.ContainsRune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ", rune(letter)) {
		return "", false
	}
	return volume + `\`, true
}

// volumeProtection asks the shell what BitLocker is doing with the volume at
// root, which must be a volume root such as `C:\`.
//
// The thread is pinned for the length of the call because COM apartments
// belong to threads rather than to goroutines: initialised on one thread and
// left on another, the calls below would be made on a thread that never
// entered an apartment.
func volumeProtection(root string) (*bool, error) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	ours, err := enterApartment()
	if err != nil {
		return nil, err
	}
	if ours {
		defer windows.CoUninitialize()
	}

	key, err := protectionPropertyKey()
	if err != nil {
		return nil, err
	}

	item, err := shellItemFor(root)
	if err != nil {
		return nil, err
	}
	defer item.release()

	value, err := item.getInt32(key)
	if err != nil {
		return nil, err
	}
	return bitLockerProtection(value), nil
}

// sFalse is the HRESULT for "this succeeded, and there was nothing to do".
const sFalse = 1

// enterApartment puts this thread into a COM apartment and says whether
// leaving it again is this call's job.
//
// A thread already in an apartment of the same kind answers S_FALSE, which is
// a success and still counts the reference this call took. One already in an
// apartment of the other kind answers RPC_E_CHANGED_MODE: the apartment is
// somebody else's, it serves for the reads here, and ending it would end it
// under whoever entered it.
func enterApartment() (ours bool, err error) {
	initErr := windows.CoInitializeEx(0, windows.COINIT_APARTMENTTHREADED)
	if initErr == nil {
		return true, nil
	}
	var errno syscall.Errno
	if !errors.As(initErr, &errno) {
		return false, errNoShellAnswer
	}
	switch uintptr(errno) {
	case sFalse:
		return true, nil
	case uintptr(windows.RPC_E_CHANGED_MODE):
		return false, nil
	}
	return false, errNoShellAnswer
}

// protectionPropertyKey resolves the property by the name the shell knows it
// by, so this file carries no GUID a reader would have to take on trust.
func protectionPropertyKey() (*propertyKey, error) {
	name, err := windows.UTF16PtrFromString(protectionPropertyName)
	if err != nil {
		return nil, errNoShellAnswer
	}
	var key propertyKey
	hr, _, _ := procPSGetPropertyKeyFromName.Call(
		uintptr(unsafe.Pointer(name)),
		uintptr(unsafe.Pointer(&key)))
	if hr != 0 {
		return nil, errNoShellAnswer
	}
	return &key, nil
}

// shellItemFor asks the shell for the object standing for a path.
func shellItemFor(path string) (*iShellItem2, error) {
	wide, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return nil, errNoShellAnswer
	}
	var item *iShellItem2
	hr, _, _ := procSHCreateItemFromParsingName.Call(
		uintptr(unsafe.Pointer(wide)),
		0, // no bind context: this resolves a path already on the filesystem.
		uintptr(unsafe.Pointer(&shellItem2IID)),
		uintptr(unsafe.Pointer(&item)))
	if hr != 0 || item == nil {
		return nil, errNoShellAnswer
	}
	return item, nil
}
