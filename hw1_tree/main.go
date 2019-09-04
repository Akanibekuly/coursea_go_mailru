package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
)

func main() {
	out := os.Stdout

	if !(len(os.Args) == 2 || len(os.Args) == 3) {
		panic("usage go run main.go . [-f]")
	}

	path := os.Args[1]
	printFiles := len(os.Args) == 3 && os.Args[2] == "-f"

	err := dirTree(out, path, printFiles)

	if err != nil {
		panic(err.Error())
	}
}

func dirTree(out io.Writer, path string, printFiles bool) error {
	return handleLevel(out, path, printFiles, "")
}

func handleLevel(out io.Writer, path string, printFiles bool, stringPrefix string) error {
	currentDirectory, err := os.Open(path)

	if err != nil {
		return err
	}

	files, err := currentDirectory.Readdir(-1)
	currentDirectory.Close()

	if err != nil {
		return err
	}

	if !printFiles {
		for i := 0; i < len(files); i++ {
			if !files[i].IsDir() {
				files = append(files[:i], files[i+1:]...)
				i--
			}
		}
	}

	sort.Slice(files, func(i, j int) bool {
		return files[i].Name() < files[j].Name()
	})

	for i, file := range files {

		if file.Name() == ".DS_Store" {
			continue
		}

		isLast := i == len(files)-1
		prefix, newPrefix := getPrefixes(stringPrefix, isLast)

		if file.IsDir() {
			out.Write([]byte(fmt.Sprintf("%s%s\n", prefix, file.Name())))
		} else {
			var size string

			if file.Size() == 0 {
				size = "empty"
			} else {
				size = fmt.Sprintf("%db", file.Size())
			}

			out.Write([]byte(fmt.Sprintf("%s%s (%s)\n", prefix, file.Name(), size)))
		}

		if file.IsDir() {
			handleLevel(out, filepath.Join(path, file.Name()), printFiles, newPrefix)
		}
	}

	return nil
}

func getPrefixes(rawPrefix string, last bool) (prefix string, newPrefix string) {
	if last {
		return rawPrefix + "└───", rawPrefix + "\t"
	}

	return rawPrefix + "├───", rawPrefix + "│\t"
}
