package spinner

var SpinnerChars = `-\|/`

type Spinner struct {
	cnt int
}

func New() *Spinner {
	return &Spinner{}
}

func (s *Spinner) Next() string {
	s.cnt++
	runes := []rune(SpinnerChars)
	return string([]rune{runes[s.cnt%len(runes)], '\b'})
}
