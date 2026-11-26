package desktop

type Options struct {
	URL         string
	Directory   string
	Version     string
	OpenBrowser bool
	Exit        func()
}
