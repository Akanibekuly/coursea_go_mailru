package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"strings"
)

type User struct {
	Browsers []string
	Company  string
	Country  string
	Email    string
	Job      string
	Name     string
	Phone    string
}

func FastSearch(out io.Writer) {
	file, err := os.Open(filePath)
	if err != nil {
		panic(err)
	}

	sc := bufio.NewScanner(file)
	if err != nil {
		panic(err)
	}

	seenBrowsers := make(map[string]bool, 100)
	uniqueBrowsers := 0
	foundUsers := new(bytes.Buffer)

	users := make([]User, 0)

	for sc.Scan() {
		line := sc.Text()
		user := User{}
		err := json.Unmarshal([]byte(line), &user)

		if err != nil {
			panic(err)
		}

		users = append(users, user)
	}

	if err := sc.Err(); err != nil {
		log.Fatalf("scan file error: %v", err)
		return
	}

	for i, user := range users {
		isAndroid := false
		isMSIE := false

		for _, browser := range user.Browsers {
			if ok := strings.Contains(browser, "Android"); ok {
				isAndroid = true

				if exists := seenBrowsers[browser]; !exists {
					seenBrowsers[browser] = true
					uniqueBrowsers++
				}
			} else if ok := strings.Contains(browser, "MSIE"); ok {
				isMSIE = true

				if exists := seenBrowsers[browser]; !exists {
					seenBrowsers[browser] = true
					uniqueBrowsers++
				}
			}
		}

		if !(isAndroid && isMSIE) {
			continue
		}

		email := strings.Replace(user.Email, "@", " [at] ", -1)
		foundUsers.WriteString(fmt.Sprintf("[%d] %s <%s>\n", i, user.Name, email))
	}

	fmt.Fprintln(out, "found users:\n"+foundUsers.String())
	fmt.Fprintln(out, "Total unique browsers", len(seenBrowsers))
}
