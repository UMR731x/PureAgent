package desktop

import (
	"context"
	"fmt"
	"log"

	"github.com/egoist/mygo"

	"pureagent/web"
)

// Run owns the Windows event loop and must run on the main goroutine.
func Run(ctx context.Context, options Options) error {
	mygo.App.SetName("PureAgent")
	mygo.App.SetVersion(options.Version)
	mygo.App.SetMenu(nil)
	resources := &workspaceResources{handler: options.Resources}
	defer resources.close()
	if err := mygo.Protocol.Handle("mygo", resources); err != nil {
		return err
	}
	defer mygo.Protocol.Unhandle("mygo")
	if !mygo.WebViewAvailable() {
		return fmt.Errorf("请先安装 Microsoft Edge WebView2 运行时，再启动 PureAgent：https://developer.microsoft.com/microsoft-edge/webview2")
	}
	icon, err := web.Files.ReadFile("icon.png")
	if err != nil {
		return err
	}
	quitting := false
	mygo.App.OnBeforeQuit(func(*mygo.QuitEvent) { quitting = true })
	mygo.App.OnQuitCanceled(func() { quitting = false })
	failures := make(chan error, 1)
	fail := func(err error) {
		select {
		case failures <- err:
		default:
		}
		mygo.App.Quit()
	}
	var stopTray func()
	done := make(chan struct{})
	defer close(done)
	mygo.App.WhenReady(func() {
		readLegacyPreferences(options.LegacyOrigin, fail, func(preload string) {
			window := mygo.NewWindow(mygo.WindowOptions{
				Title: "PureAgent", Width: 1280, Height: 860, Hidden: true,
				Page: mygo.PageOptions{PreloadScript: preload},
			})
			if err := window.SetIcon(icon); err != nil {
				fail(err)
				return
			}
			window.OnClose(func(event *mygo.CloseEvent) {
				if !quitting {
					event.PreventDefault()
					window.Hide()
				}
			})
			stopTray, err = startTray(options, func() { go showWorkspace(window) }, mygo.App.Quit)
			if err != nil {
				fail(err)
				return
			}
			loadWorkspace(window, options, fail)
			go waitForShutdown(ctx, done, window, options.Restart)
		})
	})
	err = mygo.App.Run()
	if stopTray != nil {
		stopTray()
	}
	select {
	case err := <-failures:
		return err
	default:
		return err
	}
}

func loadWorkspace(window *mygo.Window, options Options, fail func(error)) {
	loaded := false
	window.Page().OnDidFinishLoad(func() {
		if loaded || window.Page().URL() == "about:blank" {
			return
		}
		loaded = true
		if err := options.Ready(); err != nil {
			fail(err)
			return
		}
		// Older installers launch with SW_HIDE. Override it only on the first show;
		// ShowInactive would reset a maximized window to normal bounds on later calls.
		window.ShowInactive()
		window.Focus()
	})
	window.Page().OnDidFailLoad(func(err *mygo.LoadError) {
		if !loaded {
			fail(fmt.Errorf("无法打开工作台：%w", err))
		}
	})
	// WebView2 initialization failures reach pending page calls, not navigation events.
	// Await initialization before navigating, so evaluation cannot race navigation.
	go func() {
		if _, err := window.Page().Eval("document.readyState"); err != nil {
			fail(fmt.Errorf("无法启动 WebView2：%w", err))
			return
		}
		if err := window.Page().LoadURL("mygo://localhost/"); err != nil {
			fail(err)
		}
	}()
}

func showWorkspace(window *mygo.Window) {
	if window.IsMinimized() {
		window.Restore()
	}
	window.Show()
	window.Focus()
}

func waitForShutdown(ctx context.Context, done <-chan struct{}, window *mygo.Window, restart <-chan struct{}) {
	select {
	case <-ctx.Done():
	case <-restart:
		if _, err := window.Page().Eval("window.dispatchEvent(new Event('pureagent-before-update'))"); err != nil {
			log.Printf("无法保存更新前的输入草稿：%v", err)
		}
	case <-done:
		return
	}
	mygo.App.Quit()
}
