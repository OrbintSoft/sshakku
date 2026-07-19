//go:build darwin

package keys

/*
#cgo LDFLAGS: -framework Security -framework CoreFoundation
#include <CoreFoundation/CoreFoundation.h>
#include <Security/Security.h>
*/
import "C"

import (
	"fmt"
	"unsafe"
)

// secSuccess/secItemNotFound are Go-typed copies of Security.framework's
// errSecSuccess/errSecItemNotFound OSStatus constants, so status comparisons
// below don't need a repeated C.OSStatus(...) conversion at every call site.
var (
	secSuccess      = C.OSStatus(C.errSecSuccess)
	secItemNotFound = C.OSStatus(C.errSecItemNotFound)
)

// DarwinKeychainClient implements KeychainClient over Security.framework's
// generic-password API. A passphrase only ever crosses into a CFDataRef
// handed straight to the framework — never a subprocess's argv or stdin,
// the exposure shelling out to the `security` CLI's `-w` flag would have.
type DarwinKeychainClient struct{}

// cfString creates a CFStringRef from a Go string. The caller owns the
// returned reference and must release it via cfRelease.
func cfString(s string) C.CFStringRef {
	b := []byte(s)
	var p *C.UInt8
	if len(b) > 0 {
		p = (*C.UInt8)(unsafe.Pointer(&b[0]))
	}
	return C.CFStringCreateWithBytes(C.kCFAllocatorDefault, p, C.CFIndex(len(b)), C.kCFStringEncodingUTF8, 0)
}

// cfData creates a CFDataRef from a Go string's bytes. The caller owns the
// returned reference and must release it via cfRelease.
func cfData(s string) C.CFDataRef {
	b := []byte(s)
	var p *C.UInt8
	if len(b) > 0 {
		p = (*C.UInt8)(unsafe.Pointer(&b[0]))
	}
	return C.CFDataCreate(C.kCFAllocatorDefault, p, C.CFIndex(len(b)))
}

// cfRelease releases any CF*Ref, regardless of its specific cgo-generated Go
// type, by routing through unsafe.Pointer.
func cfRelease(ref unsafe.Pointer) {
	if ref != nil {
		C.CFRelease(C.CFTypeRef(ref))
	}
}

// goCFString converts a CFStringRef to a Go string; a nil ref yields "".
func goCFString(s C.CFStringRef) string {
	if unsafe.Pointer(s) == nil {
		return ""
	}
	n := C.CFStringGetLength(s)
	size := C.CFStringGetMaximumSizeForEncoding(n, C.kCFStringEncodingUTF8) + 1
	buf := make([]byte, int(size))
	if C.CFStringGetCString(s, (*C.char)(unsafe.Pointer(&buf[0])), size, C.kCFStringEncodingUTF8) == 0 {
		return ""
	}
	return C.GoString((*C.char)(unsafe.Pointer(&buf[0])))
}

// goCFData converts a CFDataRef to a Go string.
func goCFData(d C.CFDataRef) string {
	n := C.CFDataGetLength(d)
	if n == 0 {
		return ""
	}
	p := C.CFDataGetBytePtr(d)
	return string(C.GoBytes(unsafe.Pointer(p), C.int(n)))
}

// newQuery builds the CFMutableDictionaryRef every generic-password
// operation starts from: class=genericPassword, service, account. The
// caller owns the returned reference and must release it via cfRelease.
func newQuery(account, service string) C.CFMutableDictionaryRef {
	d := C.CFDictionaryCreateMutable(C.kCFAllocatorDefault, 0, &C.kCFTypeDictionaryKeyCallBacks, &C.kCFTypeDictionaryValueCallBacks)
	C.CFDictionarySetValue(d, unsafe.Pointer(C.kSecClass), unsafe.Pointer(C.kSecClassGenericPassword))

	svc := cfString(service)
	defer cfRelease(unsafe.Pointer(svc))
	C.CFDictionarySetValue(d, unsafe.Pointer(C.kSecAttrService), unsafe.Pointer(svc))

	acc := cfString(account)
	defer cfRelease(unsafe.Pointer(acc))
	C.CFDictionarySetValue(d, unsafe.Pointer(C.kSecAttrAccount), unsafe.Pointer(acc))

	return d
}

// secError formats a non-zero OSStatus as an error, using
// SecCopyErrorMessageString for a human-readable message when available.
func secError(op string, status C.OSStatus) error {
	msg := C.SecCopyErrorMessageString(status, nil)
	if unsafe.Pointer(msg) == nil {
		return fmt.Errorf("%s: OSStatus %d", op, int(status))
	}
	defer cfRelease(unsafe.Pointer(msg))
	return fmt.Errorf("%s: %s (OSStatus %d)", op, goCFString(msg), int(status))
}

// Find implements KeychainClient.
func (DarwinKeychainClient) Find(account, service string) (string, bool, error) {
	q := newQuery(account, service)
	defer cfRelease(unsafe.Pointer(q))
	C.CFDictionarySetValue(q, unsafe.Pointer(C.kSecReturnData), unsafe.Pointer(C.kCFBooleanTrue))
	C.CFDictionarySetValue(q, unsafe.Pointer(C.kSecMatchLimit), unsafe.Pointer(C.kSecMatchLimitOne))

	var result C.CFTypeRef
	status := C.SecItemCopyMatching(C.CFDictionaryRef(q), &result)
	if status == secItemNotFound {
		return "", false, nil
	}
	if status != secSuccess {
		return "", false, secError("keychain lookup", status)
	}
	defer cfRelease(unsafe.Pointer(result))
	return goCFData(C.CFDataRef(result)), true, nil
}

// Add implements KeychainClient.
func (DarwinKeychainClient) Add(account, service, label, passphrase string) error {
	attrs := newQuery(account, service)
	defer cfRelease(unsafe.Pointer(attrs))

	lbl := cfString(label)
	defer cfRelease(unsafe.Pointer(lbl))
	C.CFDictionarySetValue(attrs, unsafe.Pointer(C.kSecAttrLabel), unsafe.Pointer(lbl))

	val := cfData(passphrase)
	defer cfRelease(unsafe.Pointer(val))
	C.CFDictionarySetValue(attrs, unsafe.Pointer(C.kSecValueData), unsafe.Pointer(val))

	status := C.SecItemAdd(C.CFDictionaryRef(attrs), nil)
	if status != secSuccess {
		return secError("keychain add", status)
	}
	return nil
}

// Update implements KeychainClient.
func (DarwinKeychainClient) Update(account, service, passphrase string) error {
	q := newQuery(account, service)
	defer cfRelease(unsafe.Pointer(q))

	update := C.CFDictionaryCreateMutable(C.kCFAllocatorDefault, 0, &C.kCFTypeDictionaryKeyCallBacks, &C.kCFTypeDictionaryValueCallBacks)
	defer cfRelease(unsafe.Pointer(update))
	val := cfData(passphrase)
	defer cfRelease(unsafe.Pointer(val))
	C.CFDictionarySetValue(update, unsafe.Pointer(C.kSecValueData), unsafe.Pointer(val))

	status := C.SecItemUpdate(C.CFDictionaryRef(q), C.CFDictionaryRef(update))
	if status != secSuccess {
		return secError("keychain update", status)
	}
	return nil
}

// Delete implements KeychainClient. A missing item reports success, matching
// KeychainBackend.Delete's documented idempotence.
func (DarwinKeychainClient) Delete(account, service string) error {
	q := newQuery(account, service)
	defer cfRelease(unsafe.Pointer(q))

	status := C.SecItemDelete(C.CFDictionaryRef(q))
	if status == secItemNotFound {
		return nil
	}
	if status != secSuccess {
		return secError("keychain delete", status)
	}
	return nil
}

// List implements KeychainClient.
func (DarwinKeychainClient) List(account string) ([]string, error) {
	d := C.CFDictionaryCreateMutable(C.kCFAllocatorDefault, 0, &C.kCFTypeDictionaryKeyCallBacks, &C.kCFTypeDictionaryValueCallBacks)
	defer cfRelease(unsafe.Pointer(d))
	C.CFDictionarySetValue(d, unsafe.Pointer(C.kSecClass), unsafe.Pointer(C.kSecClassGenericPassword))

	acc := cfString(account)
	defer cfRelease(unsafe.Pointer(acc))
	C.CFDictionarySetValue(d, unsafe.Pointer(C.kSecAttrAccount), unsafe.Pointer(acc))

	C.CFDictionarySetValue(d, unsafe.Pointer(C.kSecReturnAttributes), unsafe.Pointer(C.kCFBooleanTrue))
	C.CFDictionarySetValue(d, unsafe.Pointer(C.kSecMatchLimit), unsafe.Pointer(C.kSecMatchLimitAll))

	var result C.CFTypeRef
	status := C.SecItemCopyMatching(C.CFDictionaryRef(d), &result)
	if status == secItemNotFound {
		return nil, nil
	}
	if status != secSuccess {
		return nil, secError("keychain list", status)
	}
	defer cfRelease(unsafe.Pointer(result))

	arr := C.CFArrayRef(result)
	n := C.CFArrayGetCount(arr)
	services := make([]string, 0, int(n))
	for i := C.CFIndex(0); i < n; i++ {
		item := C.CFDictionaryRef(C.CFArrayGetValueAtIndex(arr, i))
		svc := C.CFStringRef(C.CFDictionaryGetValue(item, unsafe.Pointer(C.kSecAttrService)))
		if s := goCFString(svc); s != "" {
			services = append(services, s)
		}
	}
	return services, nil
}

var _ KeychainClient = DarwinKeychainClient{}
