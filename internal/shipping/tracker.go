package shipping

import (
	"fmt"
	"math/rand"
)

// TrackingID generates a tracking ID seeded from the address string.
func TrackingID(salt string) string {
	return fmt.Sprintf("%c%c-%d%s-%d%s",
		randomLetter(),
		randomLetter(),
		len(salt),
		randomDigits(3),
		len(salt)/2,
		randomDigits(7),
	)
}

func randomLetter() rune {
	return rune(65 + rand.Intn(25)) // A-Z
}

func randomDigits(n int) string {
	buf := make([]byte, n)
	for i := range buf {
		buf[i] = '0' + byte(rand.Intn(10))
	}
	return string(buf)
}