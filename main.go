package main

import (
	"embed"
	"fmt"
	"github.com/davidbyttow/govips/v2/vips"
	"html/template"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/gabriel-vasile/mimetype"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"

	"github.com/danvergara/morphos/pkg/files"
)

const (
	uploadFileFormField = "uploadFile"
)

var (
	//go:embed all:templates
	templatesHTML embed.FS

	//go:embed all:static
	staticFiles embed.FS
	// Upload path.
	// It is a variable now, which means that can be
	// cofigurable through a environment variable.
	uploadPath string
)

func init() {
	uploadPath = os.Getenv("TMP_DIR")
	if uploadPath == "" {
		uploadPath = "/tmp"
	}
}

type ConvertedFile struct {
	Filename string
	FileType string
}

func index(w http.ResponseWriter, _ *http.Request) {
	tmpls := []string{
		"templates/base.tmpl",
		"templates/partials/htmx.tmpl",
		"templates/partials/style.tmpl",
		"templates/partials/nav.tmpl",
		"templates/partials/form.tmpl",
		"templates/partials/modal.tmpl",
		"templates/partials/js.tmpl",
	}

	tmpl, err := template.ParseFS(templatesHTML, tmpls...)
	if err != nil {
		log.Printf("error ocurred parsing templates: %v", err)
		renderError(w, "INTERNAL_ERROR", http.StatusInternalServerError)
		return
	}

	err = tmpl.ExecuteTemplate(w, "base", nil)
	if err != nil {
		log.Printf("error ocurred executing template: %v", err)
		renderError(w, "INTERNAL_ERROR", http.StatusInternalServerError)
		return
	}
}

func handleUploadFile(w http.ResponseWriter, r *http.Request) {
	var (
		convertedFile     []byte
		convertedFilePath string
		convertedFileName string
		err               error
	)

	// parse and validate file and post parameters.
	file, fileHeader, err := r.FormFile(uploadFileFormField)
	if err != nil {
		log.Printf("error ocurred getting file from form: %v", err)
		renderError(w, "INVALID_FILE", http.StatusBadRequest)
		return
	}
	defer file.Close()

	fileBytes, err := io.ReadAll(file)
	if err != nil {
		log.Printf("error ocurred reading file: %v", err)
		renderError(w, "INVALID_FILE", http.StatusBadRequest)
		return
	}

	targetFileSubType := r.FormValue("input_format")

	detectedFileType := mimetype.Detect(fileBytes)

	fileType, subType, err := files.TypeAndSupType(detectedFileType.String())
	if err != nil {
		log.Printf("error occurred getting type and subtype from mimetype: %v", err)
		renderError(w, "INVALID_FILE", http.StatusBadRequest)
		return
	}

	fileFactory, err := files.BuildFactory(fileType, fileHeader.Filename)
	if err != nil {
		log.Printf("error occurred while getting a file factory: %v", err)
		renderError(w, "INVALID_FILE", http.StatusBadRequest)
		return
	}

	f, err := fileFactory.NewFile(subType)
	if err != nil {
		log.Printf("error occurred getting the file object: %v", err)
		renderError(w, "INVALID_FILE", http.StatusBadRequest)
		return
	}

	targetFileType := files.SupportedFileTypes()[targetFileSubType]
	convertedFile, err = f.ConvertTo(
		cases.Title(language.English).String(targetFileType),
		targetFileSubType,
		fileBytes,
	)
	if err != nil {
		log.Printf("error ocurred while converting image %v", err)
		renderError(w, "INTERNAL_ERROR", http.StatusInternalServerError)
		return
	}

	if fileType == "application" {
		targetFileSubType = "zip"
	}

	convertedFileName = filename(fileHeader.Filename, targetFileSubType)
	convertedFilePath = filepath.Join(uploadPath, convertedFileName)

	newFile, err := os.Create(convertedFilePath)
	if err != nil {
		log.Printf("error occurred converting file: %v", err)
		renderError(w, "CANT_WRITE_FILE", http.StatusInternalServerError)
		return
	}
	defer newFile.Close()
	if _, err := newFile.Write(convertedFile); err != nil {
		log.Printf("error occurred writing file: %v", err)
		renderError(w, "CANT_WRITE_FILE", http.StatusInternalServerError)
		return
	}

	tmpls := []string{
		"templates/partials/card_file.tmpl",
		"templates/partials/modal.tmpl",
	}

	tmpl, err := template.ParseFS(templatesHTML, tmpls...)
	if err != nil {
		log.Printf("error occurred parsing template files: %v", err)
		renderError(w, "INTERNAL_ERROR", http.StatusInternalServerError)
		return
	}

	convertedFileMimeType := mimetype.Detect(convertedFile)

	convertedFileType, _, err := files.TypeAndSupType(convertedFileMimeType.String())
	if err != nil {
		log.Printf("error occurred getting the file type of the result file: %v", err)
		renderError(w, "INTERNAL_ERROR", http.StatusInternalServerError)
		return
	}

	err = tmpl.ExecuteTemplate(
		w,
		"content",
		ConvertedFile{Filename: convertedFileName, FileType: convertedFileType},
	)
	if err != nil {
		log.Printf("error occurred executing template files: %v", err)
		renderError(w, "INTERNAL_ERROR", http.StatusInternalServerError)
		return
	}
}

func handleFileFormat(w http.ResponseWriter, r *http.Request) {

	file, _, err := r.FormFile(uploadFileFormField)
	if err != nil {
		log.Printf("error ocurred getting file from form: %v", err)
		renderError(w, "INVALID_FILE", http.StatusBadRequest)
		return
	}

	fileBytes, err := io.ReadAll(file)
	if err != nil {
		log.Printf("error occurred executing template files: %v", err)
		renderError(w, "INVALID_FILE", http.StatusBadRequest)
		return
	}

	detectedFileType := mimetype.Detect(fileBytes)

	templates := []string{
		"templates/partials/form.tmpl",
	}

	fileType, subType, err := files.TypeAndSupType(detectedFileType.String())
	if err != nil {
		log.Printf("error occurred getting type and subtype from mimetype: %v", err)
		renderError(w, "INVALID_FILE", http.StatusBadRequest)
		return
	}

	fileFactory, err := files.BuildFactory(fileType, "")
	if err != nil {
		log.Printf("error occurred while getting a file factory: %v", err)
		renderError(w, "INVALID_FILE", http.StatusBadRequest)
		return
	}

	f, err := fileFactory.NewFile(subType)
	if err != nil {
		log.Printf("error occurred getting the file object: %v", err)
		renderError(w, "INTERNAL_ERROR", http.StatusInternalServerError)
		return
	}

	tmpl, err := template.ParseFS(templatesHTML, templates...)
	if err = tmpl.ExecuteTemplate(w, "format-elements", f.SupportedFormats()); err != nil {
		log.Printf("error occurred parsing template files: %v", err)
		renderError(w, "INTERNAL_ERROR", http.StatusInternalServerError)
		return
	}
}

func handleModal(w http.ResponseWriter, r *http.Request) {
	filename := r.URL.Query().Get("filename")
	filetype := r.URL.Query().Get("filetype")

	tmpls := []string{
		"templates/partials/active_modal.tmpl",
	}

	tmpl, err := template.ParseFS(templatesHTML, tmpls...)
	if err != nil {
		log.Printf("error occurred parsing template files: %v", err)
		renderError(w, "INTERNAL_ERROR", http.StatusInternalServerError)
		return
	}

	if err = tmpl.ExecuteTemplate(w, "content", ConvertedFile{Filename: filename, FileType: filetype}); err != nil {
		log.Printf("error occurred executing template files: %v", err)
		renderError(w, "INTERNAL_ERROR", http.StatusInternalServerError)
		return
	}
}

func main() {
	// start vips
	vips.Startup(&vips.Config{ConcurrencyLevel: runtime.NumCPU()})
	defer vips.Shutdown()

	port := os.Getenv("MORPHOS_PORT")
	// default port.
	if port == "" {
		port = "8080"
	}

	r := chi.NewRouter()
	r.Use(middleware.Logger)

	fsUpload := http.FileServer(http.Dir(uploadPath))

	var staticFS = http.FS(staticFiles)
	fs := http.FileServer(staticFS)

	r.Handle("/static/*", fs)
	r.Handle("/files/*", http.StripPrefix("/files", fsUpload))
	r.Get("/", index)
	r.Post("/upload", handleUploadFile)
	r.Post("/format", handleFileFormat)
	r.Get("/modal", handleModal)

	http.ListenAndServe(fmt.Sprintf(":%s", port), r)
}

func renderError(w http.ResponseWriter, message string, statusCode int) {
	w.WriteHeader(statusCode)
	w.Write([]byte(message))
}

func fileNameWithoutExtension(fileName string) string {
	return strings.TrimSuffix(filepath.Base(fileName), filepath.Ext(fileName))
}

func filename(filename, extension string) string {
	return fmt.Sprintf("%s.%s", fileNameWithoutExtension(filename), extension)
}
