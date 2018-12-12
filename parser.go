package bot

import (
	"errors"
	"regexp"
	"strings"

	"github.com/mattn/go-shellwords"
	unidecode "github.com/mozillazg/go-unidecode"
)

var (
	re = regexp.MustCompile("\\s+") // Matches one or more spaces
)

// var specialChars = []rune{'\'', '"', '(', ')'}
// var specialPairs = []rune{'\'', '"', ')', '('}

// CharWash helps to check a string for solo chars
type CharWash struct {
	Char   rune
	Pair   rune
	REChar *regexp.Regexp
	REPair *regexp.Regexp
}

// var cw = make(map[rune]*CharWash)
var cw = map[rune]*CharWash{
	'\'': &CharWash{'\'', '\'', regexp.MustCompile("'"), regexp.MustCompile("'")},
	'"':  &CharWash{'"', '"', regexp.MustCompile(`"`), regexp.MustCompile(`"`)},
	'(':  &CharWash{'(', ')', regexp.MustCompile("\\("), regexp.MustCompile("\\)")},
}

// func init() {
// 	for i := range specialChars {
// 		fmt.Printf("specialChar: %c\n", specialChars[i])
// 		scS := string(specialChars[i])
// 		spS := string(specialPairs[i])
// 		cw[specialChars[i]] = &CharWash{
// 			Char:   specialChars[i],
// 			Pair:   specialPairs[i],
// 			REChar: regexp.MustCompile(scS),
// 			REPair: regexp.MustCompile(spS),
// 		}
// 	}
// }

func cleanString(s string) string {
	old := []byte(s)
	ret := old
	for _, c := range cw {
		if c.Char == c.Pair {
			// count the number of instance in the string
			// if count % 2 > 0 then remove the last one
			found := c.REChar.FindAllIndex(ret, -1)
			if len(found)%2 > 0 {
				idx := found[len(found)-1][0]
				ret = append(ret[:idx], ret[idx+1:]...)
			}
		} else {
			// count the number of instance of each of the Char and Pair
			// cal abs(c-p) and remove that number of wich ever one has more
			foundC := c.REChar.FindAllIndex(ret, -1)
			foundP := c.REPair.FindAllIndex(ret, -1)
			removeNum := len(foundC) - len(foundP)
			var remove [][]int
			if removeNum > 0 {
				remove = foundC
			}
			if removeNum < 0 {
				remove = foundP
				removeNum = removeNum * -1
			}

			for i := 0; i < removeNum; i++ {
				idx := remove[len(remove)-removeNum][0]
				ret = append(ret[:idx], ret[idx+1:]...)

			}
		}
	}
	// if string(old) != string(ret) {
	// 	fmt.Printf("clean string old: %s new: %s\n", old, ret)
	// }
	return string(ret)
}

func parse(s string, channel *ChannelData, user *User) (*Cmd, error) {
	c := &Cmd{Raw: s}
	s = strings.TrimSpace(s)

	if !strings.HasPrefix(s, CmdPrefix) {
		return nil, nil
	}

	c.Channel = strings.TrimSpace(channel.Channel)
	c.ChannelData = channel
	c.User = user

	// Trim the prefix and extra spaces
	c.Message = strings.TrimPrefix(s, CmdPrefix)
	c.Message = strings.TrimSpace(c.Message)

	// check if we have the command and not only the prefix
	if c.Message == "" {
		return nil, nil
	}

	// get the command
	pieces := strings.SplitN(c.Message, " ", 2)
	c.Command = strings.ToLower(unidecode.Unidecode(pieces[0]))

	if len(pieces) > 1 {
		// get the arguments and remove extra spaces
		c.RawArgs = strings.TrimSpace(pieces[1])
		parsedArgs, err := shellwords.Parse(unidecode.Unidecode(cleanString(c.RawArgs)))
		if err != nil {
			return nil, errors.New("Error parsing arguments: " + err.Error())
		}
		c.Args = parsedArgs
	}

	c.MessageData = &Message{
		Text: c.Message,
	}

	return c, nil
}
