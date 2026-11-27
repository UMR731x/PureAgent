package desktop

import (
	"encoding/json"
	"fmt"
	"log"

	"github.com/egoist/mygo"
)

// LoadHTML intercepts this document inside WebView2; it never contacts the old
// HTTP address. The hidden page can read this app's previous localStorage origin.
func readLegacyPreferences(origin string, fail func(error), ready func(string)) {
	previous := mygo.NewWindow(mygo.WindowOptions{Title: "PureAgent", Hidden: true})
	finished := false
	finish := func(preferences map[string]string) {
		if finished {
			return
		}
		finished = true
		encoded, _ := json.Marshal(preferences)
		ready(`(() => {
  if (location.origin !== 'http://mygo.localhost' || localStorage.getItem('pureagent-portless-ui')) return;
  for (const [key, value] of Object.entries(` + string(encoded) + ` || {})) {
    if (localStorage.getItem(key) === null) localStorage.setItem(key, value);
  }
  localStorage.setItem('pureagent-portless-ui', '1');
})();`)
		// Create the workspace first so closing this temporary page does not quit MyGo.
		previous.Destroy()
	}
	previous.Page().OnDidFinishLoad(func() {
		if previous.Page().URL() != origin || finished {
			return
		}
		go func() {
			preferences, err := mygo.EvalAs[map[string]string](previous.Page(), `Object.fromEntries(
  ['pureagent-appearance', 'pureagent-model', 'pureagent-desktop-update']
    .map(key => [key, localStorage.getItem(key)]).filter(([, value]) => value !== null)
)`)
			if err != nil {
				log.Printf("无法读取旧界面偏好：%v", err)
			}
			mygo.App.WhenReady(func() { finish(preferences) })
		}()
	})
	previous.Page().OnDidFailLoad(func(err *mygo.LoadError) {
		log.Printf("无法读取旧界面偏好：%v", err)
		finish(nil)
	})
	go func() {
		if _, err := previous.Page().Eval("document.readyState"); err != nil {
			fail(fmt.Errorf("无法启动 WebView2：%w", err))
			return
		}
		previous.Page().LoadHTML(`<!doctype html><meta charset="utf-8"><link rel="icon" href="data:,">`, origin)
	}()
}
