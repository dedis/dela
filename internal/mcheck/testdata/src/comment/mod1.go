package comment

// This line should be ok, this is why there is no "want" at the end of it,
// which is the way of testing an analyzer.

// This line is too long and should raise an error because it exceed the 80 chars limit // want ""

/*
This line is not too long and should raise an error because it exceed the 80
chars
*/

//go:generate this line should be ignore even if it's too long because it started with a go:generate
