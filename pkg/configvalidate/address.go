package configvalidate

import (
	"errors"
	"net"
	"strconv"
	"strings"
)

func ValidateAddress(value string, allowEmptyHost bool) error {
	host, portText, err := net.SplitHostPort(value)
	if err != nil || (!allowEmptyHost && host == "") {
		return errors.New("must be host:port")
	}
	port, err := strconv.ParseUint(portText, 10, 16)
	if err != nil || port == 0 {
		return errors.New("port must be between 1 and 65535")
	}
	if host != "" && net.ParseIP(host) == nil && !validHostname(host) {
		return errors.New("host is invalid")
	}
	return nil
}

func validHostname(host string) bool {
	host = strings.TrimSuffix(host, ".")
	if host == "" || len(host) > 253 {
		return false
	}
	for _, label := range strings.Split(host, ".") {
		if len(label) == 0 || len(label) > 63 || !isAlphaNumeric(label[0]) || !isAlphaNumeric(label[len(label)-1]) {
			return false
		}
		for index := 1; index < len(label)-1; index++ {
			if !isAlphaNumeric(label[index]) && label[index] != '-' {
				return false
			}
		}
	}
	return true
}

func isAlphaNumeric(value byte) bool {
	return value >= 'a' && value <= 'z' || value >= 'A' && value <= 'Z' || value >= '0' && value <= '9'
}
