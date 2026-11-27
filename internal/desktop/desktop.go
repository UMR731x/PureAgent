package desktop

type Options struct {
	URL       string
	Directory string
	Version   string
	Ready     func() error
	Restart   <-chan struct{}
}
