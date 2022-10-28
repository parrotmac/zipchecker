package main

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"html/template"
	"io/ioutil"
	"log"
	"net/http"
	"strings"
)

const baseHTML = `<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8" />
<title>zip file checker</title>
<style>
html, body {
  padding: 0;
  margin: 0;
  height: 100%;
}

body {
  background-color: #211a12;
  color: #f5a9a9;
  font-family: Helvetica, Arial, sans-serif;
  width: 100%;
  height: 100%;
  display: flex;
  flex-direction: column;
  align-items: center;
}

a, a:link, a:visited {
  color: #EAEAEA;
}

.file-listing {
  display: flex;
  flex-direction: column;
}

.file-detail {
  margin: 0.125rem 0;
  padding: 0.125rem 1rem;
  display: flex;
  flex-direction: column;
  border-bottom: 1px dashed;
}

.file-detail:hover {
  background-color: #4c362e;
}

.file-detail > span {
  font-size: 15pt;
}

.file-metadata {
  margin: 0.125rem;
  display: flex;
  flex-direction: row;
  justify-content: space-between;
}

.file-size {
  font-size: 11pt;
}

.type-hint {
  font-size: 11pt;
}

.drop-target {
  margin: 1rem;
}
</style>
</head>
<body>
<div class="drop-target">
  <span class="headline-text">Drag A File Anywhere</span>
  <!-- <span class="hint-text">Or Click To Choose A File</span> -->
</div>
<div class="file-listing"></div>
<script>
function humanFileSize(size) {
    const i = size === 0 ? 0 : Math.floor( Math.log(size) / Math.log(1024) );
    return ( size / Math.pow(1024, i) ).toFixed(2) * 1 + ' ' + ['B', 'kB', 'MB', 'GB', 'TB'][i];
};

function updateFileList(fileListing) {
  const target = document.querySelector(".file-listing");
  target.innerHTML = "";

  for (let i = 0; i < fileListing.length; i++) {
    const filename = fileListing[i].filename;
    const filesize = fileListing[i].size;
    const typeHints = fileListing[i].type_hints.join(",");

    const fileDetail = document.createElement("div");
    fileDetail.classList.add("file-detail");

    const headerText = document.createElement("code");
    headerText.textContent = filename;

    const detailSection = document.createElement("div");
    detailSection.classList.add("file-metadata");
    const sizeText = document.createElement("span");
    sizeText.classList.add("file-size");
    sizeText.textContent = humanFileSize(filesize);
    const typeHintText = document.createElement("span");
    typeHintText.classList.add("type-hint");
    typeHintText.textContent = typeHints;

    detailSection.appendChild(sizeText);
    detailSection.appendChild(typeHintText);

    fileDetail.appendChild(headerText);
    fileDetail.appendChild(detailSection);

    target.appendChild(fileDetail);
  }
  //target.innerHTML = fileDetail.outerHTML;
}

function getFileDetails(fileObj, callback) {
  const req = fetch('/check', {
    method: 'post',
    body: fileObj,
  }).then(resp => {
    if (parseInt(resp.status / 100) != 2) {
        callback(undefined, resp.body);
	return;
    } else {
        resp.json().then(jsonBody => {
	  console.log("File Data", jsonBody);
	  callback(jsonBody);
	}).catch(err => {
	    callback(undefined, err);
	});
    }
  }).catch(err => {
    callback(undefined, err);
  });
}

function dropHandler(ev) {
  console.log('File(s) dropped');
  ev.preventDefault();
  if (ev.dataTransfer.items) {
    // Use DataTransferItemList interface to access the file(s)
    for (var i = 0; i < ev.dataTransfer.items.length; i++) {
      // If dropped items aren't files, reject them
      if (ev.dataTransfer.items[i].kind === 'file') {
        var file = ev.dataTransfer.items[i].getAsFile();
	getFileDetails(file, function(resp, err) {
	    if (err) {
	        alert(err);
		return;
	    }
	    updateFileList(resp);
	})
        console.log('... file[' + i + '].name = ' + file.name);
      }
    }
  } else {
    // Use DataTransfer interface to access the file(s)
    for (var i = 0; i < ev.dataTransfer.files.length; i++) {
	getFileDetails(file, function(resp, err) {
	    if (err) {
	        alert(err);
		return;
	    }
	    updateFileList(resp);
	})
      console.log('... file[' + i + '].name = ' + ev.dataTransfer.files[i].name);
    }
  }
  return false;
}

const dropzone = document.querySelector("body");
dropzone.addEventListener('drop', dropHandler, true);
dropzone.addEventListener('dragenter', function(ev) {
  ev.preventDefault();
  return false;
});
dropzone.addEventListener('dragover', function(ev) {
  ev.preventDefault();
  return false;
});

</script>
<!-- Garabge: {{.Garbage}} -->
</body>
</html>
`

func index(w http.ResponseWriter, req *http.Request) {
	t, err := template.New("index").Parse(baseHTML)
	if err != nil {
		w.WriteHeader(500)
		w.Write([]byte(err.Error()))
		return
	}
	err = t.Execute(w, struct {
		Garbage string
	}{
		Garbage: ":(",
	})
	if err != nil {
		w.WriteHeader(500)
		w.Write([]byte(err.Error()))
		return
	}
	return
}

func headers(w http.ResponseWriter, req *http.Request) {
	for name, headers := range req.Header {
		for _, h := range headers {
			fmt.Fprintf(w, "%v: %v\n", name, h)
		}
	}
}

func checkStaticMagic(offset int, subjectSlice []byte, magicSlice []byte) bool {
	magicLen := len(magicSlice)
	if len(subjectSlice)-offset < magicLen {
		// Not enough bytes
		return false
	}
	for i, magicByte := range magicSlice {
		subjOffset := i + offset
		if subjectSlice[subjOffset] != magicByte {
			return false
		}
	}
	return true
}

func readZipFile(zf *zip.File) ([]byte, error) {
	f, err := zf.Open()
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return ioutil.ReadAll(f)
}

type fileDesc struct {
	Filename  string   `json:"filename"`
	Size      int64    `json:"size"`
	TypeHints []string `json:"type_hints"`
}

func zipCheck(w http.ResponseWriter, req *http.Request) {
	body, err := ioutil.ReadAll(req.Body)
	if err != nil {
		log.Println("Failed to read body", err)
		return
	}

	zipReader, err := zip.NewReader(bytes.NewReader(body), int64(len(body)))
	if err != nil {
		log.Println("Failed to read zip into zip reader", err)
		return
	}

	var fileDescriptions []fileDesc

	// Read all the files from zip archive
	for _, zipFile := range zipReader.File {
		if strings.HasSuffix(zipFile.Name, "/") {
			log.Println("Has suffix /, skipping")
			continue
		}
		fmt.Println("Reading file:", zipFile.Name)
		unzippedFileBytes, err := readZipFile(zipFile)
		if err != nil {
			log.Println("Falied to inspect ZIP file:", err)
			continue
		}

		typeHints := []string{}

		// PDFs (if not using UTF-8 encoding, and using predictable 0-byte offset
		if checkStaticMagic(0, unzippedFileBytes, []byte("%PDF")) {
			typeHints = append(typeHints, "PDF")
		}

		// Windows EXE
		if checkStaticMagic(0, unzippedFileBytes, []byte("MZ")) {
			typeHints = append(typeHints, "Windows Executable")
		}

		// ELF Executable
		if checkStaticMagic(0, unzippedFileBytes, []byte{
			0x7F,
			0x45,
			0x4C,
			0x46,
		}) {
			typeHints = append(typeHints, "ELF Executable")
		}

		// Zip Files
		if checkStaticMagic(0, unzippedFileBytes, []byte{0x50, 0x4B, 0x03}) ||
			checkStaticMagic(0, unzippedFileBytes, []byte{0x50, 0x4B, 0x04}) ||
			checkStaticMagic(0, unzippedFileBytes, []byte{0x50, 0x4B, 0x05}) {
			typeHints = append(typeHints, "ZIP")
		}

		// doc/xls/ppt (legacy binary formats)
		if checkStaticMagic(0, unzippedFileBytes, []byte{
			0xD0,
			0xCF,
			0x11,
			0xE0,
			0xA1,
			0xB1,
			0x1A,
			0xE1,
		}) {
			typeHints = append(typeHints, "MS-Office")
		}

		// DMG
		if checkStaticMagic(0, unzippedFileBytes, []byte{
			0x6B,
			0x6F,
			0x6C,
			0x79,
		}) {
			typeHints = append(typeHints, "Apple Disk Image")
		}

		fileDescriptions = append(fileDescriptions, fileDesc{
			Filename:  zipFile.Name,
			Size:      int64(len(unzippedFileBytes)),
			TypeHints: typeHints,
		})
	}

	respData, err := json.Marshal(fileDescriptions)
	if err != nil {
		w.WriteHeader(500)
		w.Write([]byte(err.Error()))
		return
	}
	_, err = w.Write(respData)
	if err != nil {
		log.Println(err)
	}
}

func main() {

	http.HandleFunc("/", index)
	http.HandleFunc("/headers", headers)
	http.HandleFunc("/check", zipCheck)

	http.ListenAndServe(":8090", nil)
}
