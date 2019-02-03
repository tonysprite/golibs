package cookie

import (
	"golang.org/x/crypto/pbkdf2"
	"runtime"

	"crypto/aes"
	"crypto/cipher"
	"crypto/sha1"
	"database/sql"
	"fmt"
	"log"
	"os/exec"
	"os/user"
	"strings"

	_ "github.com/mattn/go-sqlite3"
)

// Inspiration
// http://n8henrie.com/2013/11/use-chromes-cookies-for-easier-downloading-with-python-requests/
// https://gist.github.com/dacort/bd6a5116224c594b14db

// This path would be changed by environment even same OS
var cookieBaseDir = map[string]string{
	"darwin": "%s/Library/Application Support/Google/Chrome/Default/Cookies", //mac
	"linux":  "%s/.config/google-chrome/Default/Cookies",
}

// Chromium Mac os_crypt:  http://dacort.me/1ynPMgx
var (
	salt       = "saltysalt"
	iv         = "                "
	length     = 16
	password   = ""
	iterations = 1003
)

// Cookie - Items for a cookie
type Cookie struct {
	Domain         string
	Key            string
	Value          string
	EncryptedValue []byte
}

// DecryptedValue - Get the unencrypted value of a Chrome cookie
func (c *Cookie) DecryptedValue() string {
	if c.Value > "" {
		return c.Value
	}

	if len(c.EncryptedValue) > 0 {
		encryptedValue := c.EncryptedValue[3:]
		return decryptValue(encryptedValue)
	}

	return ""
}

func callerSample() {
	domain := "localhost"
	PrintCookies(domain)

	_ = GetValue(domain, "key")
}

func PrintCookies(url string) {
	password = getPassword()

	for _, cookie := range getCookies(url) {
		fmt.Printf("%s/%s: %s\n", cookie.Domain, cookie.Key, cookie.DecryptedValue())
	}
	//localhost/cookiename: xxxxxx
}

func GetValue(url, key string) string {
	password = getPassword()

	for _, cookie := range getCookies(url) {
		if cookie.Domain == url && cookie.Key == key {
			return cookie.DecryptedValue()
		}
	}
	return ""
}

func GetAllValue(url string) map[string]string {
	password = getPassword()

	cookies := make(map[string]string)
	for _, cookie := range getCookies(url) {
		cookies[cookie.Key] = cookie.DecryptedValue()
	}
	return cookies
}

func decryptValue(encryptedValue []byte) string {
	key := pbkdf2.Key([]byte(password), []byte(salt), iterations, length, sha1.New)
	block, err := aes.NewCipher(key)
	if err != nil {
		log.Fatal(err)
	}

	decrypted := make([]byte, len(encryptedValue))
	cbc := cipher.NewCBCDecrypter(block, []byte(iv))
	cbc.CryptBlocks(decrypted, encryptedValue)

	plainText, err := aesStripPadding(decrypted)
	if err != nil {
		fmt.Println("Error decrypting:", err)
		return ""
	}
	return string(plainText)
}

// In the padding scheme the last <padding length> bytes
// have a value equal to the padding length, always in (1,16]
func aesStripPadding(data []byte) ([]byte, error) {
	if len(data)%length != 0 {
		return nil, fmt.Errorf("decrypted data block length is not a multiple of %d", length)
	}
	paddingLen := int(data[len(data)-1])
	if paddingLen > 16 {
		return nil, fmt.Errorf("invalid last block padding length: %d", paddingLen)
	}
	return data[:len(data)-paddingLen], nil
}

func getPassword() string {
	parts := strings.Fields("security find-generic-password -wga Chrome")
	cmd := parts[0]
	parts = parts[1:len(parts)]

	out, err := exec.Command(cmd, parts...).Output()
	if err != nil {
		log.Fatal("error finding password ", err)
	}

	return strings.Trim(string(out), "\n")
}

func getCookies(domain string) (cookies []Cookie) {
	usr, _ := user.Current()

	var cookiesFile string
	if val, ok := cookieBaseDir[runtime.GOOS]; ok {
		cookiesFile = fmt.Sprintf(val, usr.HomeDir)
	} else {
		log.Fatal("os is not set in coolie path")
	}

	db, err := sql.Open("sqlite3", cookiesFile)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	rows, err := db.Query("SELECT name, value, host_key, encrypted_value FROM cookies WHERE host_key like ?", fmt.Sprintf("%%%s%%", domain))
	if err != nil {
		log.Fatal(err)
	}

	defer rows.Close()
	for rows.Next() {
		var name, value, hostKey string
		var encryptedValue []byte
		rows.Scan(&name, &value, &hostKey, &encryptedValue)
		cookies = append(cookies, Cookie{hostKey, name, value, encryptedValue})
	}

	return
}
