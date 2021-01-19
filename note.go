package main

import (
	"fmt"
	"strconv"
	"unicode"
)

type Note int

func (n Note) IsWhite() bool {
	degree := int(n) % 12
	return map[int]bool{0: true, 2: true, 4: true, 5: true, 7: true, 9: true, 11: true}[degree]
}

func NewNote(noteStr string) (Note, error) {
	i := 0
	note := Note(0)
	runes := []rune(noteStr)
	if len(runes) < 2 {
		return 0, fmt.Errorf("too short")
	}
	degree := runes[i]
	if !unicode.IsLetter(degree) {
		return 0, fmt.Errorf("degree not a letter")
	}
	degree = unicode.ToLower(degree)
	if !map[rune]bool{'a': true, 'b': true, 'c': true, 'd': true, 'e': true, 'f': true, 'g': true}[degree] {
		return 0, fmt.Errorf("invalid degree")
	}
	note = map[rune]Note{'c': 0, 'd': 2, 'e': 4, 'f': 5, 'g': 7, 'a': 9, 'b': 11}[degree]
	i++
	if !unicode.IsDigit(runes[i]) {
		sharpFlat := runes[i]
		if unicode.IsLetter(sharpFlat) {
			sharpFlat = unicode.ToLower(sharpFlat)
		}
		switch sharpFlat {
		case 's':
			note++
		case '#':
			note++
		case 'f':
			note--
		case 'b':
			note--
		default:
			return 0, fmt.Errorf("invalid sharp/flat")
		}
		i++
	}
	octave, err := strconv.Atoi(string(runes[i:]))
	if err != nil {
		return 0, err
	}
	note += Note(12 * octave)

	return note, nil
}
