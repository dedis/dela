package testdata

// This line should be ok, this is why there is no "want" at the end of it,
// which is the way of testing an analyzer.

// This line is too long and should raise an error because it exceed the 80 chars limit // want ""

/*
This line is not too long and should raise an error because it exceed the 80
chars
*/
