package leakdetector

type PatternInfo struct {
	Name    string
	Pattern string
	Type    string
}

type Match struct {
	Pattern string
	Type    string
}
