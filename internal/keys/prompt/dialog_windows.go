//go:build windows

package prompt

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"sync"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"

	"github.com/OrbintSoft/sshakku/internal/run"
)

// nativePrompterName is what gui_prompter calls this dialog, and what a message
// about it names. The dialogs on the other platforms are named after the program
// that draws them; there is no such program here, because this one is drawn by
// SSHakku itself — which is the whole point of it. Nothing has to be installed,
// no interpreter is started, and no execution policy has anything to say about
// a window that is not a script.
const nativePrompterName = "native"

// errBudgetRanOut is the window taken down because the time a person was given
// to answer ran out. It is emphatically not a dismissal: nobody decided
// anything, so the key is not given up on and the asking may go on elsewhere.
var errBudgetRanOut = errors.New("the passphrase box was taken down before anybody answered it")

// errNoWindow is a window that could not be put on the screen at all.
var errNoWindow = errors.New("the passphrase box could not be opened")

// Win32 names as the headers spell them, so this code can be checked against the
// documentation rather than against itself.
const (
	wsChild     = 0x40000000
	wsVisible   = 0x10000000
	wsCaption   = 0x00C00000
	wsSysMenu   = 0x00080000
	wsTabStop   = 0x00010000
	wsBorder    = 0x00800000
	wsExTopMost = 0x00000008

	esPassword      = 0x0020
	esAutoHScroll   = 0x0080
	bsDefPushButton = 0x0001

	wmDestroy       = 0x0002
	wmClose         = 0x0010
	wmGetText       = 0x000D
	wmGetTextLength = 0x000E
	wmSetFont       = 0x0030
	wmCommand       = 0x0111

	idOK     = 1
	idCancel = 2

	smCXScreen = 0
	smCYScreen = 1

	swShow = 5

	idcArrow         = 32512
	colorBtnFace     = 15
	fwNormal         = 400
	defaultCharset   = 1
	cleartypeQuality = 5
)

var (
	gdi32 = windows.NewLazySystemDLL("gdi32.dll")

	procRegisterClassEx     = user32.NewProc("RegisterClassExW")
	procCreateWindowEx      = user32.NewProc("CreateWindowExW")
	procDefWindowProc       = user32.NewProc("DefWindowProcW")
	procDestroyWindow       = user32.NewProc("DestroyWindow")
	procGetMessage          = user32.NewProc("GetMessageW")
	procTranslateMessage    = user32.NewProc("TranslateMessage")
	procDispatchMessage     = user32.NewProc("DispatchMessageW")
	procPostQuitMessage     = user32.NewProc("PostQuitMessage")
	procIsDialogMessage     = user32.NewProc("IsDialogMessageW")
	procSendMessage         = user32.NewProc("SendMessageW")
	procPostMessage         = user32.NewProc("PostMessageW")
	procSetFocus            = user32.NewProc("SetFocus")
	procSetForegroundWindow = user32.NewProc("SetForegroundWindow")
	procShowWindow          = user32.NewProc("ShowWindow")
	procGetSystemMetrics    = user32.NewProc("GetSystemMetrics")
	procSetWindowText       = user32.NewProc("SetWindowTextW")
	procLoadCursor          = user32.NewProc("LoadCursorW")
	procAdjustWindowRect    = user32.NewProc("AdjustWindowRect")
	procGetDpiForSystem     = user32.NewProc("GetDpiForSystem")

	procCreateFont   = gdi32.NewProc("CreateFontW")
	procDeleteObject = gdi32.NewProc("DeleteObject")
)

// wndClassEx mirrors WNDCLASSEXW field for field and in order; rect and msgRec
// do the same for RECT and MSG. A field added, removed or reordered here is not
// a compile error anywhere — it is a call reading the wrong bytes.
type wndClassEx struct {
	Size       uint32
	Style      uint32
	WndProc    uintptr
	ClsExtra   int32
	WndExtra   int32
	Instance   windows.Handle
	Icon       windows.Handle
	Cursor     windows.Handle
	Background windows.Handle
	MenuName   *uint16
	ClassName  *uint16
	IconSm     windows.Handle
}

type rect struct{ Left, Top, Right, Bottom int32 }

type msgRec struct {
	Hwnd    windows.HWND
	Message uint32
	WParam  uintptr
	LParam  uintptr
	Time    uint32
	Pt      struct{ X, Y int32 }
}

// liveWindows maps a window to the state its message handler acts on. The
// pointer is kept here rather than in the window's own user data because a Go
// pointer handed to the system and read back is a pointer the runtime is no
// longer accounting for.
var liveWindows sync.Map // windows.HWND -> *passphraseWindow.

// windowFor is the state behind a live window, or false once there is none: the
// system keeps delivering messages to a window that is being taken down, and
// the last of them have nothing left to act on.
func windowFor(hwnd windows.HWND) (*passphraseWindow, bool) {
	v, ok := liveWindows.Load(hwnd)
	if !ok {
		return nil, false
	}
	w, ok := v.(*passphraseWindow)
	return w, ok
}

// errClass is whatever the one class registration came to, kept so that every
// window after the first is told the same thing rather than a nil error.
var (
	classOnce sync.Once
	errClass  error
)

// className is this window's class. It is registered once per process.
var className = windows.StringToUTF16Ptr("SSHakkuPassphraseWindow")

// NativePrompter asks for a passphrase in a window SSHakku draws itself.
type NativePrompter struct {
	// Timeout bounds the window. It is a person's budget, not a machine's, but
	// still finite: a box nobody answers must not strand the shell that raised
	// it. Zero selects run.DefaultInteractiveTimeout.
	Timeout time.Duration
}

// Prompt shows the passphrase box for keyname and returns what was typed.
//
// A dismissal comes back as ErrCanceled, which is a decision. A budget that ran
// out does not: it comes back as an error, because nobody decided anything and
// the question may still be worth asking somewhere else.
func (p NativePrompter) Prompt(ctx context.Context, keyname string) (string, error) {
	timeout := p.Timeout
	if timeout <= 0 {
		timeout = run.DefaultInteractiveTimeout
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	w := &passphraseWindow{
		keyname: keyname,
		ready:   make(chan windows.HWND, 1),
		done:    make(chan struct{}),
	}
	go w.run()

	// The watcher lives exactly as long as this call: it is what closes the
	// window when the caller gives up, and it must not outlive the answer.
	watching := make(chan struct{})
	defer close(watching)

	select {
	case hwnd := <-w.ready:
		go func() {
			select {
			case <-ctx.Done():
				// Asking the window to close rather than destroying it from
				// here: a window belongs to the thread that made it, and only
				// that thread may take it down.
				_, _, _ = procPostMessage.Call(uintptr(hwnd), wmClose, 0, 0)
			case <-watching:
			}
		}()
	case <-w.done:
	}

	<-w.done

	switch {
	case w.err != nil:
		return "", w.err
	case w.answered:
		return w.pass, nil
	case ctx.Err() != nil:
		return "", fmt.Errorf("%w: %w", errBudgetRanOut, ctx.Err())
	default:
		return "", ErrCanceled
	}
}

// Name is what to call this prompter in a message.
func (p NativePrompter) Name() string { return nativePrompterName }

// Available reports whether this prompter can ask here, and it always can:
// there is nothing to install and nothing to find. Whether the session it would
// draw on has a screen is a different question and not this type's to answer.
func (p NativePrompter) Available(context.Context) bool { return true }

var _ Prompter = NativePrompter{}

// passphraseWindow is one window and the answer it collected.
//
// Everything but ready and done is written on the window's own thread and read
// only after done is closed, which is what publishes it to the caller.
type passphraseWindow struct {
	keyname string

	ready chan windows.HWND
	done  chan struct{}

	pass     string
	answered bool
	err      error

	edit windows.HWND
	font windows.Handle
}

// run owns the window from beginning to end, on a thread of its own.
//
// The thread is locked because a window belongs to the thread that created it:
// its messages are delivered there and nowhere else, so a goroutine that
// wandered onto another thread would stop receiving them.
func (w *passphraseWindow) run() {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	defer close(w.done)

	if err := registerWindowClass(); err != nil {
		w.err = err
		return
	}

	hwnd, err := w.create()
	if err != nil {
		w.err = err
		return
	}
	liveWindows.Store(hwnd, w)
	defer liveWindows.Delete(hwnd)
	defer w.releaseFont()

	w.ready <- hwnd
	w.pump(hwnd)
}

// pump runs the window's message loop until the window is gone.
//
// IsDialogMessage is what makes the keyboard behave the way a person expects of
// a dialog — Tab moving between the controls, Return meaning the default button
// and Escape meaning the other one — without any of it being written here.
func (w *passphraseWindow) pump(hwnd windows.HWND) {
	var m msgRec
	for {
		ret, _, _ := procGetMessage.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0)
		// 0 is WM_QUIT and -1 is an error; both mean this loop is over.
		if ret == 0 || int32(ret) == -1 {
			return
		}
		if handled, _, _ := procIsDialogMessage.Call(uintptr(hwnd), uintptr(unsafe.Pointer(&m))); handled != 0 {
			continue
		}
		_, _, _ = procTranslateMessage.Call(uintptr(unsafe.Pointer(&m)))
		_, _, _ = procDispatchMessage.Call(uintptr(unsafe.Pointer(&m)))
	}
}

// accept takes what was typed out of the edit control and clears it, so the
// passphrase stops existing in a window that has not been destroyed yet.
func (w *passphraseWindow) accept() {
	n, _, _ := procSendMessage.Call(uintptr(w.edit), wmGetTextLength, 0, 0)
	buf := make([]uint16, n+1)
	_, _, _ = procSendMessage.Call(uintptr(w.edit), wmGetText, uintptr(len(buf)), uintptr(unsafe.Pointer(&buf[0])))
	w.pass = windows.UTF16ToString(buf)
	for i := range buf {
		buf[i] = 0
	}
	empty := windows.StringToUTF16Ptr("")
	_, _, _ = procSetWindowText.Call(uintptr(w.edit), uintptr(unsafe.Pointer(empty)))
	w.answered = true
}

// releaseFont gives back the font this window made for itself.
func (w *passphraseWindow) releaseFont() {
	if w.font != 0 {
		_, _, _ = procDeleteObject.Call(uintptr(w.font))
		w.font = 0
	}
}

// passphraseWndProc handles what the window is told. Everything it does not
// recognise goes to the system's own handling, which is what draws the frame,
// moves the window and closes it when its button is pressed.
func passphraseWndProc(hwnd, message, wParam, lParam uintptr) uintptr {
	switch message {
	case wmCommand:
		w, ok := windowFor(windows.HWND(hwnd))
		if !ok {
			break
		}
		switch wParam & 0xFFFF {
		case idOK:
			w.accept()
			_, _, _ = procDestroyWindow.Call(hwnd)
			return 0
		case idCancel:
			_, _, _ = procDestroyWindow.Call(hwnd)
			return 0
		}
	case wmClose:
		_, _, _ = procDestroyWindow.Call(hwnd)
		return 0
	case wmDestroy:
		_, _, _ = procPostQuitMessage.Call(0)
		return 0
	}
	ret, _, _ := procDefWindowProc.Call(hwnd, message, wParam, lParam)
	return ret
}

// moduleHandle is this program's own module, which a window class and a window
// are both created against. The reference count is left alone: this module is
// the running executable and is not going anywhere.
func moduleHandle() (windows.Handle, error) {
	const unchangedRefcount = 0x00000002
	var h windows.Handle
	if err := windows.GetModuleHandleEx(unchangedRefcount, nil, &h); err != nil {
		return 0, fmt.Errorf("%w: %w", errNoWindow, err)
	}
	return h, nil
}

// registerWindowClass registers this window's class, once for the process. A
// second registration of the same name fails, and a class outlives every window
// drawn from it, so there is nothing to undo afterwards.
func registerWindowClass() error {
	classOnce.Do(func() {
		instance, err := moduleHandle()
		if err != nil {
			errClass = err
			return
		}
		cursor, _, _ := procLoadCursor.Call(0, idcArrow)

		class := wndClassEx{
			Size:       uint32(unsafe.Sizeof(wndClassEx{})),
			WndProc:    windows.NewCallback(passphraseWndProc),
			Instance:   instance,
			Cursor:     windows.Handle(cursor),
			Background: windows.Handle(colorBtnFace + 1),
			ClassName:  className,
		}
		if ret, _, err := procRegisterClassEx.Call(uintptr(unsafe.Pointer(&class))); ret == 0 {
			errClass = fmt.Errorf("%w: %w", errNoWindow, err)
		}
	})
	return errClass
}

// create puts the window and its controls on the screen, and returns the window.
func (w *passphraseWindow) create() (windows.HWND, error) {
	instance, err := moduleHandle()
	if err != nil {
		return 0, err
	}

	// Sizes are written for the 96 dpi the numbers in the documentation assume
	// and scaled to whatever this screen really is, so the box is the same size
	// in front of a person whatever their display is set to.
	dpi := systemDPI()
	scale := func(v int32) int32 { return v * int32(dpi) / 96 }

	// Made without WS_VISIBLE and shown once its controls are in it: a frame
	// that appears first and fills in afterwards is one a person can see empty,
	// and can reach before there is anything in it to reach.
	clientW, clientH := scale(380), scale(132)
	frame := rect{Right: clientW, Bottom: clientH}
	style := uint32(wsCaption | wsSysMenu)
	_, _, _ = procAdjustWindowRect.Call(uintptr(unsafe.Pointer(&frame)), uintptr(style), 0)
	winW, winH := frame.Right-frame.Left, frame.Bottom-frame.Top

	screenW, _, _ := procGetSystemMetrics.Call(smCXScreen)
	screenH, _, _ := procGetSystemMetrics.Call(smCYScreen)
	x := (int32(screenW) - winW) / 2
	y := (int32(screenH) - winH) / 2

	title := windows.StringToUTF16Ptr("SSHakku")
	hwnd, _, err := procCreateWindowEx.Call(
		wsExTopMost,
		uintptr(unsafe.Pointer(className)),
		uintptr(unsafe.Pointer(title)),
		uintptr(style),
		uintptr(x), uintptr(y), uintptr(winW), uintptr(winH),
		0, 0, uintptr(instance), 0,
	)
	if hwnd == 0 {
		return 0, fmt.Errorf("%w: %w", errNoWindow, err)
	}
	window := windows.HWND(hwnd)

	w.font = w.makeFont(dpi)

	// A window missing any of these is not a window anybody can answer, so it
	// is reported rather than shown: an empty frame on the screen would be
	// worse than no frame at all.
	controls := []struct {
		class, text  string
		style        uint32
		x, y, cx, cy int32
		id           uintptr
	}{
		{"STATIC", "Enter passphrase for " + w.keyname, wsChild | wsVisible, 14, 16, 352, 20, 0},
		{"EDIT", "", wsChild | wsVisible | wsBorder | wsTabStop | esPassword | esAutoHScroll, 14, 44, 352, 24, 0},
		{"BUTTON", "OK", wsChild | wsVisible | wsTabStop | bsDefPushButton, 196, 86, 80, 26, idOK},
		{"BUTTON", "Cancel", wsChild | wsVisible | wsTabStop, 286, 86, 80, 26, idCancel},
	}
	for _, c := range controls {
		h, err := w.child(window, instance, c.class, c.text, c.style,
			scale(c.x), scale(c.y), scale(c.cx), scale(c.cy), c.id)
		if err != nil {
			_, _, _ = procDestroyWindow.Call(hwnd)
			return 0, err
		}
		if c.class == "EDIT" {
			w.edit = h
		}
	}

	// Nothing here was started from the keyboard, so the window takes the focus
	// itself and puts the caret where the typing is meant to go. Without this
	// the first characters of a passphrase land in whatever had the focus.
	_, _, _ = procShowWindow.Call(hwnd, swShow)
	_, _, _ = procSetForegroundWindow.Call(hwnd)
	_, _, _ = procSetFocus.Call(uintptr(w.edit))
	return window, nil
}

// child creates one control on the window and gives it the window's font.
//
// The two strings are held in variables for the length of the call rather than
// converted inside the argument list: Call takes uintptrs, so a pointer written
// straight into it is a number as far as the compiler is concerned, and nothing
// keeps the string it points at alive while the system reads it.
func (w *passphraseWindow) child(parent windows.HWND, instance windows.Handle,
	class, text string, style uint32, x, y, cx, cy int32, id uintptr,
) (windows.HWND, error) {
	classPtr, err := windows.UTF16PtrFromString(class)
	if err != nil {
		return 0, fmt.Errorf("%w: %w", errNoWindow, err)
	}
	textPtr, err := windows.UTF16PtrFromString(text)
	if err != nil {
		return 0, fmt.Errorf("%w: %w", errNoWindow, err)
	}

	h, _, callErr := procCreateWindowEx.Call(
		0,
		uintptr(unsafe.Pointer(classPtr)),
		uintptr(unsafe.Pointer(textPtr)),
		uintptr(style),
		uintptr(x), uintptr(y), uintptr(cx), uintptr(cy),
		uintptr(parent), id, uintptr(instance), 0,
	)
	runtime.KeepAlive(classPtr)
	runtime.KeepAlive(textPtr)
	if h == 0 {
		return 0, fmt.Errorf("%w: the %s control: %w", errNoWindow, class, callErr)
	}
	if w.font != 0 {
		_, _, _ = procSendMessage.Call(h, wmSetFont, uintptr(w.font), 1)
	}
	return windows.HWND(h), nil
}

// makeFont builds the font the controls are drawn in. Without one they inherit
// the system's ancient bitmap face, which is how a window that is otherwise
// correct still manages to look broken.
func (w *passphraseWindow) makeFont(dpi uint32) windows.Handle {
	height := -int32(12) * int32(dpi) / 96
	face := windows.StringToUTF16Ptr("Segoe UI")
	h, _, _ := procCreateFont.Call(
		uintptr(height), 0, 0, 0, fwNormal, 0, 0, 0,
		defaultCharset, 0, 0, cleartypeQuality, 0,
		uintptr(unsafe.Pointer(face)),
	)
	return windows.Handle(h)
}

// systemDPI is what this screen is set to, or the 96 every measurement in the
// documentation assumes where the system is too old to be asked.
func systemDPI() uint32 {
	if err := procGetDpiForSystem.Find(); err != nil {
		return 96
	}
	dpi, _, _ := procGetDpiForSystem.Call()
	if dpi == 0 {
		return 96
	}
	return uint32(dpi)
}
