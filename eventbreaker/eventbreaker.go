package eventbreaker

import "regexp"

type KnownEventBreakers struct {
	regexExp []regexp.Regexp
}

/*
	NewEventBreakers Initializes a known set of log event breakers. (Note: Usually dates and ip addresses are commonly used as event breakers.)
*/
func NewEventBreakers() *KnownEventBreakers {
	eventBreakers := new(KnownEventBreakers)
	//127.0.0.1
	ipPattern, _ := regexp.Compile(`^\s*(25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?)(\.(25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?)){3}`)
	//2019-12-05 18:44:57
	datePattern1, _ := regexp.Compile(`^\s*\d{4}-[01]{1}\d{1}-[0-3]{1}\d{1} [0-2]{1}\d{1}:[0-6]{1}\d{1}:[0-6]{1}\d{1}`)
	//[2019-12-05 18:44:57]
	datePattern2, _ := regexp.Compile(`^\s*\[\d{4}-[01]{1}\d{1}-[0-3]{1}\d{1} [0-2]{1}\d{1}:[0-6]{1}\d{1}:[0-6]{1}\d{1}\]`)
	//2019-12-05T18:44:57,007
	datePattern3, _ := regexp.Compile(`^\s*\d{4}-[01]{1}\d{1}-[0-3]{1}\d{1}T[0-2]{1}\d{1}:[0-6]{1}\d{1}:[0-6]{1}\d{1}`)
	//[2019-12-05T18:44:57,007+0000]
	datePattern4, _ := regexp.Compile(`^\[\d{4}-[01]{1}\d{1}-[0-3]{1}\d{1}T[0-2]{1}\d{1}:[0-6]{1}\d{1}:[0-6]{1}\d{1},\d{3}\+\d{4}\]`)
	eventBreakers.regexExp = append(eventBreakers.regexExp, *ipPattern)
	eventBreakers.regexExp = append(eventBreakers.regexExp, *datePattern1)
	eventBreakers.regexExp = append(eventBreakers.regexExp, *datePattern2)
	eventBreakers.regexExp = append(eventBreakers.regexExp, *datePattern3)
	eventBreakers.regexExp = append(eventBreakers.regexExp, *datePattern4)
	return eventBreakers
}

func (eventBreakers KnownEventBreakers) HasKnownEventBreakerString(line string) bool {
	for _, rex := range eventBreakers.regexExp {
		if rex.MatchString(line) {
			return true
		}
	}
	return false
}
func (eventBreakers KnownEventBreakers) HasKnownEventBreakerBytes(line []byte) bool {
	return eventBreakers.HasKnownEventBreakerString(string(line))
}
