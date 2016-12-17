// Copyright 2016 Google Inc. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//    http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package compiler

import (
	"gopkg.in/yaml.v2"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
)

var file_cache map[string][]byte
var info_cache map[string]interface{}
var count int64

func FetchFile(fileurl string) ([]byte, error) {
	if file_cache == nil {
		file_cache = make(map[string][]byte, 0)
	}
	bytes, ok := file_cache[fileurl]
	if ok {
		return bytes, nil
	}
	log.Printf("fetching %s", fileurl)
	response, err := http.Get(fileurl)
	if err != nil {
		return nil, err
	} else {
		defer response.Body.Close()
		bytes, err := ioutil.ReadAll(response.Body)
		if err == nil {
			file_cache[fileurl] = bytes
		}
		return bytes, err
	}
}

// read a file and unmarshal it as a yaml.MapSlice
func ReadFile(filename string) (interface{}, error) {
	// is the filename a url?
	fileurl, _ := url.Parse(filename)
	if fileurl.Scheme != "" {
		bytes, err := FetchFile(filename)
		if err == nil {
			var info yaml.MapSlice
			err = yaml.Unmarshal(bytes, &info)
			if err == nil {
				return info, nil
			} else {
				return nil, err
			}
		} else {
			return nil, err
		}
	} else {
		// no, it's a local filename
		file, err := ioutil.ReadFile(filename)
		if err != nil {
			log.Printf("File error: %v\n", err)
			return nil, err
		}
		var info yaml.MapSlice
		yaml.Unmarshal(file, &info)
		return info, nil
	}
}

// read a file and return the fragment needed to resolve a $ref
func ReadInfoForRef(basefile string, ref string) interface{} {
	if info_cache == nil {
		info_cache = make(map[string]interface{}, 0)
	}
	{
		info, ok := info_cache[ref]
		if ok {
			return info
		}
	}

	log.Printf("%d Resolving %s", count, ref)
	count = count + 1
	basedir, _ := filepath.Split(basefile)
	parts := strings.Split(ref, "#")
	var filename string
	if parts[0] != "" {
		filename = basedir + parts[0]
	} else {
		filename = basefile
	}
	info, err := ReadFile(filename)
	if err != nil {
		log.Printf("File error: %v\n", err)
	} else {
		if len(parts) > 1 {
			path := strings.Split(parts[1], "/")
			for i, key := range path {
				if i > 0 {
					m, ok := info.(yaml.MapSlice)
					if ok {
						for _, section := range m {
							if section.Key == key {
								info = section.Value
							}
						}
					}
				}
			}
		}
	}
	info_cache[ref] = info
	return info
}
