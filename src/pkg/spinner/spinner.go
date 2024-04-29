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
	return string([]byte{SpinnerChars[s.cnt%len(SpinnerChars)], '\b'})
}
