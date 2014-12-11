// Copyright (c) 2014, B3log
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"compress/gzip"
	"flag"
	"html/template"
	"io"
	"math/rand"
	"mime"
	"net/http"
	"os"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/b3log/wide/conf"
	"github.com/b3log/wide/editor"
	"github.com/b3log/wide/event"
	"github.com/b3log/wide/file"
	"github.com/b3log/wide/i18n"
	"github.com/b3log/wide/notification"
	"github.com/b3log/wide/output"
	"github.com/b3log/wide/session"
	"github.com/b3log/wide/shell"
	"github.com/b3log/wide/util"
	"github.com/golang/glog"
)

// The only one init function in Wide.
func init() {
	confPath := flag.String("conf", "conf/wide.json", "path of wide.json")
	confIP := flag.String("ip", "", "ip to visit")
	confPort := flag.String("port", "", "port to visit")
	confServer := flag.String("server", "", "this will overwrite Wide.Server if specified")
	confStaticServer := flag.String("static_server", "", "this will overwrite Wide.StaticServer if specified")
	confContext := flag.String("context", "", "this will overwrite Wide.Context if specified")
	confChannel := flag.String("channel", "", "this will overwrite Wide.XXXChannel if specified")
	confStat := flag.Bool("stat", false, "whether report statistics periodically")
	confDocker := flag.Bool("docker", false, "whether run in a docker container")

	flag.Set("alsologtostderr", "true")
	flag.Set("stderrthreshold", "INFO")
	flag.Set("v", "3")

	flag.Parse()

	wd := util.OS.Pwd()
	if strings.HasPrefix(wd, os.TempDir()) {
		glog.Error("Don't run wide in OS' temp directory or with `go run`")

		os.Exit(-1)
	}

	i18n.Load()

	event.Load()

	conf.Load(*confPath, *confIP, *confPort, *confServer, *confStaticServer, *confContext, *confChannel, *confDocker)

	conf.FixedTimeCheckEnv()
	conf.FixedTimeSave()

	session.FixedTimeRelease()

	if *confStat {
		session.FixedTimeReport()
	}
}

// indexHandler handles request of Wide index.
func indexHandler(w http.ResponseWriter, r *http.Request) {
	httpSession, _ := session.HTTPSession.Get(r, "wide-session")
	if httpSession.IsNew {
		http.Redirect(w, r, conf.Wide.Context+"login", http.StatusFound)

		return
	}

	httpSession.Options.MaxAge = conf.Wide.HTTPSessionMaxAge
	if "" != conf.Wide.Context {
		httpSession.Options.Path = conf.Wide.Context
	}
	httpSession.Save(r, w)

	// create a Wide session
	rand.Seed(time.Now().UnixNano())
	sid := strconv.Itoa(rand.Int())
	wideSession := session.WideSessions.New(httpSession, sid)

	username := httpSession.Values["username"].(string)
	user := conf.Wide.GetUser(username)
	if nil == user {
		glog.Warningf("Not found user [%s]", username)

		http.Redirect(w, r, conf.Wide.Context+"login", http.StatusFound)

		return
	}

	locale := user.Locale

	wideSessions := session.WideSessions.GetByUsername(username)

	model := map[string]interface{}{"conf": conf.Wide, "i18n": i18n.GetAll(locale), "locale": locale,
		"session": wideSession, "latestSessionContent": user.LatestSessionContent,
		"pathSeparator": conf.PathSeparator, "codeMirrorVer": conf.CodeMirrorVer,
		"user": user, "editorThemes": conf.GetEditorThemes()}

	glog.V(3).Infof("User [%s] has [%d] sessions", username, len(wideSessions))

	t, err := template.ParseFiles("views/index.html")

	if nil != err {
		glog.Error(err)
		http.Error(w, err.Error(), 500)

		return
	}

	t.Execute(w, model)
}

// serveSingle registers the handler function for the given pattern and filename.
func serveSingle(pattern string, filename string) {
	http.HandleFunc(pattern, func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, filename)
	})
}

// startHandler handles request of start page.
func startHandler(w http.ResponseWriter, r *http.Request) {
	httpSession, _ := session.HTTPSession.Get(r, "wide-session")
	if httpSession.IsNew {
		http.Redirect(w, r, conf.Wide.Context+"login", http.StatusFound)

		return
	}

	httpSession.Options.MaxAge = conf.Wide.HTTPSessionMaxAge
	if "" != conf.Wide.Context {
		httpSession.Options.Path = conf.Wide.Context
	}
	httpSession.Save(r, w)

	username := httpSession.Values["username"].(string)
	locale := conf.Wide.GetUser(username).Locale
	userWorkspace := conf.Wide.GetUserWorkspace(username)

	sid := r.URL.Query()["sid"][0]
	wSession := session.WideSessions.Get(sid)
	if nil == wSession {
		glog.Errorf("Session [%s] not found", sid)
	}

	model := map[string]interface{}{"conf": conf.Wide, "i18n": i18n.GetAll(locale), "locale": locale,
		"username": username, "workspace": userWorkspace, "ver": conf.WideVersion, "session": wSession}

	t, err := template.ParseFiles("views/start.html")

	if nil != err {
		glog.Error(err)
		http.Error(w, err.Error(), 500)

		return
	}

	t.Execute(w, model)
}

// keyboardShortcutsHandler handles request of keyboard shortcuts page.
func keyboardShortcutsHandler(w http.ResponseWriter, r *http.Request) {
	httpSession, _ := session.HTTPSession.Get(r, "wide-session")
	if httpSession.IsNew {
		http.Redirect(w, r, conf.Wide.Context+"login", http.StatusFound)

		return
	}

	httpSession.Options.MaxAge = conf.Wide.HTTPSessionMaxAge
	if "" != conf.Wide.Context {
		httpSession.Options.Path = conf.Wide.Context
	}
	httpSession.Save(r, w)

	username := httpSession.Values["username"].(string)
	locale := conf.Wide.GetUser(username).Locale

	model := map[string]interface{}{"conf": conf.Wide, "i18n": i18n.GetAll(locale), "locale": locale}

	t, err := template.ParseFiles("views/keyboard_shortcuts.html")

	if nil != err {
		glog.Error(err)
		http.Error(w, err.Error(), 500)

		return
	}

	t.Execute(w, model)
}

// aboutHandle handles request of about page.
func aboutHandler(w http.ResponseWriter, r *http.Request) {
	httpSession, _ := session.HTTPSession.Get(r, "wide-session")
	if httpSession.IsNew {
		http.Redirect(w, r, conf.Wide.Context+"login", http.StatusFound)

		return
	}

	httpSession.Options.MaxAge = conf.Wide.HTTPSessionMaxAge
	if "" != conf.Wide.Context {
		httpSession.Options.Path = conf.Wide.Context
	}
	httpSession.Save(r, w)

	username := httpSession.Values["username"].(string)
	locale := conf.Wide.GetUser(username).Locale

	model := map[string]interface{}{"conf": conf.Wide, "i18n": i18n.GetAll(locale), "locale": locale,
		"ver": conf.WideVersion, "goos": runtime.GOOS, "goarch": runtime.GOARCH, "gover": runtime.Version()}

	t, err := template.ParseFiles("views/about.html")

	if nil != err {
		glog.Error(err)
		http.Error(w, err.Error(), 500)

		return
	}

	t.Execute(w, model)
}

// Main.
func main() {
	runtime.GOMAXPROCS(conf.Wide.MaxProcs)

	initMime()

	defer glog.Flush()

	// IDE
	http.HandleFunc(conf.Wide.Context+"/", handlerGzWrapper(indexHandler))
	http.HandleFunc(conf.Wide.Context+"/start", handlerWrapper(startHandler))
	http.HandleFunc(conf.Wide.Context+"/about", handlerWrapper(aboutHandler))
	http.HandleFunc(conf.Wide.Context+"/keyboard_shortcuts", handlerWrapper(keyboardShortcutsHandler))

	// static resources
	http.Handle(conf.Wide.Context+"/static/", http.StripPrefix(conf.Wide.Context+"/static/", http.FileServer(http.Dir("static"))))
	serveSingle("/favicon.ico", "./static/favicon.ico")

	// workspaces
	for _, user := range conf.Wide.Users {
		http.Handle(conf.Wide.Context+"/workspace/"+user.Name+"/",
			http.StripPrefix(conf.Wide.Context+"/workspace/"+user.Name+"/", http.FileServer(http.Dir(user.GetWorkspace()))))
	}

	// session
	http.HandleFunc(conf.Wide.Context+"/session/ws", handlerWrapper(session.WSHandler))
	http.HandleFunc(conf.Wide.Context+"/session/save", handlerWrapper(session.SaveContent))

	// run
	http.HandleFunc(conf.Wide.Context+"/build", handlerWrapper(output.BuildHandler))
	http.HandleFunc(conf.Wide.Context+"/run", handlerWrapper(output.RunHandler))
	http.HandleFunc(conf.Wide.Context+"/stop", handlerWrapper(output.StopHandler))
	http.HandleFunc(conf.Wide.Context+"/go/test", handlerWrapper(output.GoTestHandler))
	http.HandleFunc(conf.Wide.Context+"/go/get", handlerWrapper(output.GoGetHandler))
	http.HandleFunc(conf.Wide.Context+"/go/install", handlerWrapper(output.GoInstallHandler))
	http.HandleFunc(conf.Wide.Context+"/output/ws", handlerWrapper(output.WSHandler))

	// file tree
	http.HandleFunc(conf.Wide.Context+"/files", handlerWrapper(file.GetFiles))
	http.HandleFunc(conf.Wide.Context+"/file/refresh", handlerWrapper(file.RefreshDirectory))
	http.HandleFunc(conf.Wide.Context+"/file", handlerWrapper(file.GetFile))
	http.HandleFunc(conf.Wide.Context+"/file/save", handlerWrapper(file.SaveFile))
	http.HandleFunc(conf.Wide.Context+"/file/new", handlerWrapper(file.NewFile))
	http.HandleFunc(conf.Wide.Context+"/file/remove", handlerWrapper(file.RemoveFile))
	http.HandleFunc(conf.Wide.Context+"/file/rename", handlerWrapper(file.RenameFile))
	http.HandleFunc(conf.Wide.Context+"/file/search/text", handlerWrapper(file.SearchText))
	http.HandleFunc(conf.Wide.Context+"/file/find/name", handlerWrapper(file.Find))

	// file export/import
	http.HandleFunc(conf.Wide.Context+"/file/zip/new", handlerWrapper(file.CreateZip))
	http.HandleFunc(conf.Wide.Context+"/file/zip", handlerWrapper(file.GetZip))
	http.HandleFunc(conf.Wide.Context+"/file/upload", handlerWrapper(file.Upload))

	// editor
	http.HandleFunc(conf.Wide.Context+"/editor/ws", handlerWrapper(editor.WSHandler))
	http.HandleFunc(conf.Wide.Context+"/go/fmt", handlerWrapper(editor.GoFmtHandler))
	http.HandleFunc(conf.Wide.Context+"/autocomplete", handlerWrapper(editor.AutocompleteHandler))
	http.HandleFunc(conf.Wide.Context+"/exprinfo", handlerWrapper(editor.GetExprInfoHandler))
	http.HandleFunc(conf.Wide.Context+"/find/decl", handlerWrapper(editor.FindDeclarationHandler))
	http.HandleFunc(conf.Wide.Context+"/find/usages", handlerWrapper(editor.FindUsagesHandler))

	// shell
	http.HandleFunc(conf.Wide.Context+"/shell/ws", handlerWrapper(shell.WSHandler))
	http.HandleFunc(conf.Wide.Context+"/shell", handlerWrapper(shell.IndexHandler))

	// notification
	http.HandleFunc(conf.Wide.Context+"/notification/ws", handlerWrapper(notification.WSHandler))

	// user
	http.HandleFunc(conf.Wide.Context+"/login", handlerWrapper(session.LoginHandler))
	http.HandleFunc(conf.Wide.Context+"/logout", handlerWrapper(session.LogoutHandler))
	http.HandleFunc(conf.Wide.Context+"/signup", handlerWrapper(session.SignUpUser))
	http.HandleFunc(conf.Wide.Context+"/preference", handlerWrapper(session.PreferenceHandler))

	glog.Infof("Wide is running [%s]", conf.Wide.Server+conf.Wide.Context)

	err := http.ListenAndServe(conf.Wide.Server, nil)
	if err != nil {
		glog.Fatal(err)
	}
}

// handlerWrapper wraps the HTTP Handler for some common processes.
//
//  1. panic recover
//  2. request stopwatch
//  3. i18n
func handlerWrapper(f func(w http.ResponseWriter, r *http.Request)) func(w http.ResponseWriter, r *http.Request) {
	handler := panicRecover(f)
	handler = stopwatch(handler)
	handler = i18nLoad(handler)

	return handler
}

// handlerGzWrapper wraps the HTTP Handler for some common processes.
//
//  1. panic recover
//  2. gzip response
//  3. request stopwatch
//  4. i18n
func handlerGzWrapper(f func(w http.ResponseWriter, r *http.Request)) func(w http.ResponseWriter, r *http.Request) {
	handler := panicRecover(f)
	handler = gzipWrapper(handler)
	handler = stopwatch(handler)
	handler = i18nLoad(handler)

	return handler
}

// gzipWrapper wraps the process with response gzip.
func gzipWrapper(f func(http.ResponseWriter, *http.Request)) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") {
			f(w, r)

			return
		}

		w.Header().Set("Content-Encoding", "gzip")
		gz := gzip.NewWriter(w)
		defer gz.Close()
		gzr := gzipResponseWriter{Writer: gz, ResponseWriter: w}

		f(gzr, r)
	}
}

// i18nLoad wraps the i18n process.
func i18nLoad(handler func(w http.ResponseWriter, r *http.Request)) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		i18n.Load()

		handler(w, r)
	}
}

// stopwatch wraps the request stopwatch process.
func stopwatch(handler func(w http.ResponseWriter, r *http.Request)) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		defer func() {
			glog.V(5).Infof("[%s] [%s]", r.RequestURI, time.Since(start))
		}()

		handler(w, r)
	}
}

// panicRecover wraps the panic recover process.
func panicRecover(handler func(w http.ResponseWriter, r *http.Request)) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		defer util.Recover()

		handler(w, r)
	}
}

// initMime initializes mime types.
//
// We can't get the mime types on some OS (such as Windows XP) by default, so initializes them here.
func initMime() {
	mime.AddExtensionType(".css", "text/css")
	mime.AddExtensionType(".js", "application/x-javascript")
	mime.AddExtensionType(".json", "application/json")
}

// gzipResponseWriter represents a gzip response writer.
type gzipResponseWriter struct {
	io.Writer
	http.ResponseWriter
}

// Write writes response with appropriate 'Content-Type'.
func (w gzipResponseWriter) Write(b []byte) (int, error) {
	if "" == w.Header().Get("Content-Type") {
		// If no content type, apply sniffing algorithm to un-gzipped body.
		w.Header().Set("Content-Type", http.DetectContentType(b))
	}

	return w.Writer.Write(b)
}
