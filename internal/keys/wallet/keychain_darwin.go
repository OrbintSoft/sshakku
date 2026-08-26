//go:build darwin

package wallet

import (
	"fmt"
	"runtime"
	"sync"
	"unsafe"

	"github.com/ebitengine/purego"
)

// Every function here is a thin adapter: it marshals Go values into
// CoreFoundation types and calls a Sec* API that reads or writes the real login
// keychain — an OS side effect nothing can stand in for in a unit test. The
// KeychainClient interface is the seam; Keychain's logic is unit-tested
// against a fake in secret_keychain_test.go, and this file's real integration
// is exercised only against a live macOS keychain.
//
// The frameworks are loaded and called at run time rather than linked at build
// time, so building this package needs no C compiler and no Apple SDK — the
// binary can therefore be produced anywhere, and this file can be compiled and
// vetted from any platform even though it can only run on one. The trade is
// that no compiler checks these declarations against the real APIs: a wrong
// signature is a crash rather than a build error, and the live round trip
// against a real keychain is the only thing that can catch one.
//coverage:ignore file

// Paths dlopen accepts for the two frameworks. They name no file that exists on
// disk on a current macOS — both live in the dyld shared cache, which dlopen
// resolves these paths against.
const (
	coreFoundationPath = "/System/Library/Frameworks/CoreFoundation.framework/CoreFoundation"
	securityPath       = "/System/Library/Frameworks/Security.framework/Security"
	libSystemPath      = "/usr/lib/libSystem.B.dylib"
)

// Go-typed copies of framework constants that are compile-time literals rather
// than exported symbols, so they cannot be looked up and have to be spelled out.
const (
	cfStringEncodingUTF8 = 0x08000100
	errSecSuccess        = 0
	errSecItemNotFound   = -25300
)

// CoreFoundation and Security entry points. Every CF*Ref and CFTypeRef is an
// opaque pointer, carried as a uintptr; OSStatus is a signed 32-bit status;
// CFIndex is a signed word, which Go's int is on both architectures macOS runs
// on.
var (
	cfStringCreateWithBytes           func(alloc uintptr, bytes unsafe.Pointer, numBytes int, encoding uint32, isExternalRepresentation uint8) uintptr
	cfStringGetLength                 func(theString uintptr) int
	cfStringGetMaximumSizeForEncoding func(length int, encoding uint32) int
	cfStringGetCString                func(theString uintptr, buffer unsafe.Pointer, bufferSize int, encoding uint32) uint8
	cfDataCreate                      func(alloc uintptr, bytes unsafe.Pointer, length int) uintptr
	cfDataGetLength                   func(theData uintptr) int
	cfDataGetBytePtr                  func(theData uintptr) unsafe.Pointer
	cfDictionaryCreateMutable         func(alloc uintptr, capacity int, keyCallBacks, valueCallBacks uintptr) uintptr
	cfDictionarySetValue              func(theDict, key, value uintptr)
	cfDictionaryGetValue              func(theDict, key uintptr) uintptr
	cfArrayGetCount                   func(theArray uintptr) int
	cfArrayGetValueAtIndex            func(theArray uintptr, idx int) uintptr
	cfReleaseRef                      func(cf uintptr)

	secItemCopyMatching       func(query uintptr, result *uintptr) int32
	secItemAdd                func(attributes uintptr, result *uintptr) int32
	secItemUpdate             func(query, attributesToUpdate uintptr) int32
	secItemDelete             func(query uintptr) int32
	secCopyErrorMessageString func(status int32, reserved uintptr) uintptr

	memcpy func(dst unsafe.Pointer, src uintptr, n uintptr)
)

// Framework constants, keeping the names they have in Apple's headers so this
// code can be checked against Apple's documentation rather than against itself.
// The kSec* ones are CFStringRef keys and values; kCFBooleanTrue is the boolean
// the Sec* APIs expect for their "return this" flags.
var (
	kSecClass                uintptr
	kSecClassGenericPassword uintptr
	kSecAttrService          uintptr
	kSecAttrAccount          uintptr
	kSecAttrLabel            uintptr
	kSecValueData            uintptr
	kSecReturnData           uintptr
	kSecReturnAttributes     uintptr
	kSecMatchLimit           uintptr
	kSecMatchLimitOne        uintptr
	kSecMatchLimitAll        uintptr
	kCFBooleanTrue           uintptr

	// Unlike the constants above, these two are structs rather than pointers,
	// so what CFDictionaryCreateMutable wants is the address of the symbol
	// itself, not the value stored there.
	kCFTypeDictionaryKeyCallBacks   uintptr
	kCFTypeDictionaryValueCallBacks uintptr
)

// loadFrameworks binds everything above on first use and reports the same
// result to every later caller. It is deliberately not done at package
// initialisation: a process that never touches the keychain should not pay for
// the frameworks, and a failure to load them belongs to the operation that
// needed them rather than to starting up.
var loadFrameworks = sync.OnceValue(func() error {
	libSystem, err := purego.Dlopen(libSystemPath, purego.RTLD_LAZY|purego.RTLD_GLOBAL)
	if err != nil {
		return fmt.Errorf("loading the C library: %w", err)
	}
	coreFoundation, err := purego.Dlopen(coreFoundationPath, purego.RTLD_LAZY|purego.RTLD_GLOBAL)
	if err != nil {
		return fmt.Errorf("loading CoreFoundation: %w", err)
	}
	security, err := purego.Dlopen(securityPath, purego.RTLD_LAZY|purego.RTLD_GLOBAL)
	if err != nil {
		return fmt.Errorf("loading Security: %w", err)
	}

	purego.RegisterLibFunc(&memcpy, libSystem, "memcpy")

	purego.RegisterLibFunc(&cfStringCreateWithBytes, coreFoundation, "CFStringCreateWithBytes")
	purego.RegisterLibFunc(&cfStringGetLength, coreFoundation, "CFStringGetLength")
	purego.RegisterLibFunc(&cfStringGetMaximumSizeForEncoding, coreFoundation, "CFStringGetMaximumSizeForEncoding")
	purego.RegisterLibFunc(&cfStringGetCString, coreFoundation, "CFStringGetCString")
	purego.RegisterLibFunc(&cfDataCreate, coreFoundation, "CFDataCreate")
	purego.RegisterLibFunc(&cfDataGetLength, coreFoundation, "CFDataGetLength")
	purego.RegisterLibFunc(&cfDataGetBytePtr, coreFoundation, "CFDataGetBytePtr")
	purego.RegisterLibFunc(&cfDictionaryCreateMutable, coreFoundation, "CFDictionaryCreateMutable")
	purego.RegisterLibFunc(&cfDictionarySetValue, coreFoundation, "CFDictionarySetValue")
	purego.RegisterLibFunc(&cfDictionaryGetValue, coreFoundation, "CFDictionaryGetValue")
	purego.RegisterLibFunc(&cfArrayGetCount, coreFoundation, "CFArrayGetCount")
	purego.RegisterLibFunc(&cfArrayGetValueAtIndex, coreFoundation, "CFArrayGetValueAtIndex")
	purego.RegisterLibFunc(&cfReleaseRef, coreFoundation, "CFRelease")

	purego.RegisterLibFunc(&secItemCopyMatching, security, "SecItemCopyMatching")
	purego.RegisterLibFunc(&secItemAdd, security, "SecItemAdd")
	purego.RegisterLibFunc(&secItemUpdate, security, "SecItemUpdate")
	purego.RegisterLibFunc(&secItemDelete, security, "SecItemDelete")
	purego.RegisterLibFunc(&secCopyErrorMessageString, security, "SecCopyErrorMessageString")

	for _, c := range []struct {
		into   *uintptr
		handle uintptr
		name   string
	}{
		{&kSecClass, security, "kSecClass"},
		{&kSecClassGenericPassword, security, "kSecClassGenericPassword"},
		{&kSecAttrService, security, "kSecAttrService"},
		{&kSecAttrAccount, security, "kSecAttrAccount"},
		{&kSecAttrLabel, security, "kSecAttrLabel"},
		{&kSecValueData, security, "kSecValueData"},
		{&kSecReturnData, security, "kSecReturnData"},
		{&kSecReturnAttributes, security, "kSecReturnAttributes"},
		{&kSecMatchLimit, security, "kSecMatchLimit"},
		{&kSecMatchLimitOne, security, "kSecMatchLimitOne"},
		{&kSecMatchLimitAll, security, "kSecMatchLimitAll"},
		{&kCFBooleanTrue, coreFoundation, "kCFBooleanTrue"},
	} {
		if *c.into, err = constantValue(c.handle, c.name); err != nil {
			return err
		}
	}

	if kCFTypeDictionaryKeyCallBacks, err = purego.Dlsym(coreFoundation, "kCFTypeDictionaryKeyCallBacks"); err != nil {
		return fmt.Errorf("looking up kCFTypeDictionaryKeyCallBacks: %w", err)
	}
	if kCFTypeDictionaryValueCallBacks, err = purego.Dlsym(coreFoundation, "kCFTypeDictionaryValueCallBacks"); err != nil {
		return fmt.Errorf("looking up kCFTypeDictionaryValueCallBacks: %w", err)
	}
	return nil
})

// constantValue reads a framework constant that is exported as a global
// pointer variable: the lookup yields the variable's address, and the constant
// is the pointer stored there. The read goes through memcpy so the address
// stays an address on the C side, rather than becoming a Go pointer the garbage
// collector would be entitled to ask questions about.
func constantValue(handle uintptr, name string) (uintptr, error) {
	addr, err := purego.Dlsym(handle, name)
	if err != nil {
		return 0, fmt.Errorf("looking up %s: %w", name, err)
	}
	var value uintptr
	memcpy(unsafe.Pointer(&value), addr, unsafe.Sizeof(value))
	return value, nil
}

// DarwinKeychainClient implements KeychainClient over Security.framework's
// generic-password API. A passphrase only ever crosses into a CFDataRef
// handed straight to the framework — never a subprocess's argv or stdin,
// the exposure shelling out to the `security` CLI's `-w` flag would have.
type DarwinKeychainClient struct{}

// cfString creates a CFStringRef from a Go string. The caller owns the
// returned reference and must release it via release.
func cfString(s string) uintptr {
	b := []byte(s)
	var p unsafe.Pointer
	if len(b) > 0 {
		p = unsafe.Pointer(&b[0])
	}
	// CFStringCreateWithBytes copies what it is given — the no-copy variant is
	// a different function — so the buffer only has to outlive the call.
	ref := cfStringCreateWithBytes(0, p, len(b), cfStringEncodingUTF8, 0)
	runtime.KeepAlive(b)
	return ref
}

// cfData creates a CFDataRef from a Go string's bytes. The caller owns the
// returned reference and must release it via release.
func cfData(s string) uintptr {
	b := []byte(s)
	var p unsafe.Pointer
	if len(b) > 0 {
		p = unsafe.Pointer(&b[0])
	}
	ref := cfDataCreate(0, p, len(b))
	runtime.KeepAlive(b)
	return ref
}

// release releases any CF*Ref; a null reference is left alone.
func release(ref uintptr) {
	if ref != 0 {
		cfReleaseRef(ref)
	}
}

// goCFString converts a CFStringRef to a Go string; a null ref yields "".
func goCFString(s uintptr) string {
	if s == 0 {
		return ""
	}
	size := cfStringGetMaximumSizeForEncoding(cfStringGetLength(s), cfStringEncodingUTF8) + 1
	buf := make([]byte, size)
	ok := cfStringGetCString(s, unsafe.Pointer(&buf[0]), size, cfStringEncodingUTF8)
	runtime.KeepAlive(buf)
	if ok == 0 {
		return ""
	}
	for i, c := range buf {
		if c == 0 {
			return string(buf[:i])
		}
	}
	return string(buf)
}

// goCFData converts a CFDataRef to a Go string.
func goCFData(d uintptr) string {
	n := cfDataGetLength(d)
	if n == 0 {
		return ""
	}
	return string(unsafe.Slice((*byte)(cfDataGetBytePtr(d)), n))
}

// newQuery builds the CFMutableDictionaryRef every generic-password
// operation starts from: class=genericPassword, service, account. The
// caller owns the returned reference and must release it via release.
func newQuery(account, service string) uintptr {
	d := cfDictionaryCreateMutable(0, 0, kCFTypeDictionaryKeyCallBacks, kCFTypeDictionaryValueCallBacks)
	cfDictionarySetValue(d, kSecClass, kSecClassGenericPassword)

	svc := cfString(service)
	defer release(svc)
	cfDictionarySetValue(d, kSecAttrService, svc)

	acc := cfString(account)
	defer release(acc)
	cfDictionarySetValue(d, kSecAttrAccount, acc)

	return d
}

// keychainError is a Security framework call that refused. The status is what
// the framework said, and it is carried rather than only printed so a caller
// can tell one refusal from another — a locked keychain from a denied item —
// without matching on the sentence. The message is the framework's own where
// it has one for that status, and absent where it has not.
type keychainError struct {
	op      string
	status  int32
	message string
}

func (e keychainError) Error() string {
	if e.message == "" {
		return fmt.Sprintf("%s: OSStatus %d", e.op, e.status)
	}
	return fmt.Sprintf("%s: %s (OSStatus %d)", e.op, e.message, e.status)
}

// secError formats a non-zero OSStatus as an error, using
// SecCopyErrorMessageString for a human-readable message when available.
func secError(op string, status int32) error {
	msg := secCopyErrorMessageString(status, 0)
	if msg == 0 {
		return keychainError{op: op, status: status}
	}
	defer release(msg)
	return keychainError{op: op, status: status, message: goCFString(msg)}
}

// Find implements KeychainClient.
func (DarwinKeychainClient) Find(account, service string) (string, bool, error) {
	if err := loadFrameworks(); err != nil {
		return "", false, err
	}
	q := newQuery(account, service)
	defer release(q)
	cfDictionarySetValue(q, kSecReturnData, kCFBooleanTrue)
	cfDictionarySetValue(q, kSecMatchLimit, kSecMatchLimitOne)

	var result uintptr
	status := secItemCopyMatching(q, &result)
	if status == errSecItemNotFound {
		return "", false, nil
	}
	if status != errSecSuccess {
		return "", false, secError("keychain lookup", status)
	}
	defer release(result)
	return goCFData(result), true, nil
}

// Add implements KeychainClient.
func (DarwinKeychainClient) Add(account, service, label, passphrase string) error {
	if err := loadFrameworks(); err != nil {
		return err
	}
	attrs := newQuery(account, service)
	defer release(attrs)

	lbl := cfString(label)
	defer release(lbl)
	cfDictionarySetValue(attrs, kSecAttrLabel, lbl)

	val := cfData(passphrase)
	defer release(val)
	cfDictionarySetValue(attrs, kSecValueData, val)

	if status := secItemAdd(attrs, nil); status != errSecSuccess {
		return secError("keychain add", status)
	}
	return nil
}

// Update implements KeychainClient.
func (DarwinKeychainClient) Update(account, service, passphrase string) error {
	if err := loadFrameworks(); err != nil {
		return err
	}
	q := newQuery(account, service)
	defer release(q)

	update := cfDictionaryCreateMutable(0, 0, kCFTypeDictionaryKeyCallBacks, kCFTypeDictionaryValueCallBacks)
	defer release(update)
	val := cfData(passphrase)
	defer release(val)
	cfDictionarySetValue(update, kSecValueData, val)

	if status := secItemUpdate(q, update); status != errSecSuccess {
		return secError("keychain update", status)
	}
	return nil
}

// Delete implements KeychainClient. A missing item reports success, matching
// Keychain.Delete's documented idempotence.
func (DarwinKeychainClient) Delete(account, service string) error {
	if err := loadFrameworks(); err != nil {
		return err
	}
	q := newQuery(account, service)
	defer release(q)

	status := secItemDelete(q)
	if status == errSecItemNotFound {
		return nil
	}
	if status != errSecSuccess {
		return secError("keychain delete", status)
	}
	return nil
}

// List implements KeychainClient.
func (DarwinKeychainClient) List(account string) ([]string, error) {
	if err := loadFrameworks(); err != nil {
		return nil, err
	}
	d := cfDictionaryCreateMutable(0, 0, kCFTypeDictionaryKeyCallBacks, kCFTypeDictionaryValueCallBacks)
	defer release(d)
	cfDictionarySetValue(d, kSecClass, kSecClassGenericPassword)

	acc := cfString(account)
	defer release(acc)
	cfDictionarySetValue(d, kSecAttrAccount, acc)

	cfDictionarySetValue(d, kSecReturnAttributes, kCFBooleanTrue)
	cfDictionarySetValue(d, kSecMatchLimit, kSecMatchLimitAll)

	var result uintptr
	status := secItemCopyMatching(d, &result)
	if status == errSecItemNotFound {
		return nil, nil
	}
	if status != errSecSuccess {
		return nil, secError("keychain list", status)
	}
	defer release(result)

	n := cfArrayGetCount(result)
	services := make([]string, 0, n)
	for i := range n {
		item := cfArrayGetValueAtIndex(result, i)
		if s := goCFString(cfDictionaryGetValue(item, kSecAttrService)); s != "" {
			services = append(services, s)
		}
	}
	return services, nil
}

var _ KeychainClient = DarwinKeychainClient{}
