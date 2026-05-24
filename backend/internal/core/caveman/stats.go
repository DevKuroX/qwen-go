package caveman

// Stats records whether caveman fired and at what level — used by the request
// log so the dashboard can render a chip.
type Stats struct {
	Enabled bool
	Level   string
}
