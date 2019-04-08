package main

import (
	"context"
	"errors"
	"fmt"
	"html/template"
	"log"
	"math/rand"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path"
	"path/filepath"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/render"
)

type templateset struct {
	store map[string]*template.Template
}

type style struct {
	Stylesheets []string
	Headline    string
	Blurb       string
}

type styleset []style

func init() {
	rand.Seed(time.Now().Unix())
}

func main() {
	cwd, err := os.Getwd()
	if err != nil {
		log.Fatalf("unable to locate the current working directory: %v", err)
	}

	templatePath := path.Join(cwd, "templates")
	log.Printf("searching for templates in %q", templatePath)

	ts, err := newTemplateSet(templatePath)
	if err != nil {
		log.Fatalf("unable to compile templates: %v", err)
	}

	log.Printf("found %d templates", len(ts.store))
	for name := range ts.store {
		log.Printf("found template %s", name)
	}

	listener, err := newListener()
	if err != nil {
		log.Fatalf("unable to open listener: %v", err)
	}

	router := gin.Default()
	router.HandleMethodNotAllowed = true
	router.HTMLRender = ts
	router.Use(clacksOverhead)

	router.Static("/static", "./static")

	router.GET("/", func(c *gin.Context) {
		c.HTML(http.StatusOK, "index.html", nil)
	})

	router.Any("/auth", appendSlash)
	router.Any("/auth/", error501)
	router.Any("/v2", appendSlash)
	router.Any("/v2/", error501)

	router.NoRoute(func(c *gin.Context) {
		c.HTML(http.StatusNotFound, "404.html", gin.H{
			"style": styles.randomStyle(),
		})
	})

	serve(router, listener)
}

func clacksOverhead(c *gin.Context) {
	c.Header("X-Clacks-Overhead", "GNU Terry Pratchet")
	c.Next()
}

func newListener() (net.Listener, error) {
	var network, addr string

	socket, foundSocket := os.LookupEnv("SOCKET")
	port, foundPort := os.LookupEnv("PORT")

	switch {
	case foundSocket && foundPort:
		log.Panicf("found both SOCKET=%q and PORT=%q environment variables", socket, port)
	case foundSocket:
		log.Printf("listening on unix://%s", socket)
		network = "unix"
		addr = socket
	case foundPort:
		log.Printf("listening on tcp://0.0.0.0:%s", port)
		network = "tcp"
		addr = ":" + port
	default:
		log.Println("unable to locate SOCKET or PORT environment variable")
		log.Println("listening on tcp://127.0.0.1:5000")
		network = "tcp4"
		addr = "127.0.0.1:5000"
	}

	return net.Listen(network, addr)
}

func serve(handler *gin.Engine, listener net.Listener) {
	defer log.Println("terminating")

	server := http.Server{Handler: handler}
	go func() {
		err := server.Serve(listener)
		if err == http.ErrServerClosed {
			return
		}
		log.Fatal(err)
	}()

	signalch := make(chan os.Signal, 1)
	signal.Notify(signalch, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)
	defer signal.Reset(syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)

	for sig := range signalch {
		log.Printf("got signal %s", sig)
		if sig == syscall.SIGHUP {
			log.Printf("reloading templates")
			if ts, err := newTemplateSet("./templates"); err == nil {
				handler.HTMLRender = ts
				log.Printf("templates reloaded")
			} else {
				log.Printf("failed to rebuild templates: %v", err)
			}
		} else {
			break
		}
	}

	log.Println("shutting down...")
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		log.Fatal(err)
	}
}

func newTemplateSet(templatesDir string) (*templateset, error) {
	templates := make(map[string]*template.Template)

	layouts, err := filepath.Glob(templatesDir + "/layouts/*.html")
	if err != nil {
		return nil, err
	}

	includes, err := filepath.Glob(templatesDir + "/includes/*.html")
	if err != nil {
		return nil, err
	}

	for _, layout := range layouts {
		files := append(includes, layout)
		name := filepath.Base(layout)
		templates[name] = template.Must(template.ParseFiles(files...))
	}

	ts := templateset{templates}
	return &ts, nil
}

func (ts *templateset) Locate(name string) (*template.Template, error) {
	if tmpl, ok := ts.store[name]; ok {
		return tmpl, nil
	}

	return nil, fmt.Errorf("template %s does not exist", name)
}

func (ts *templateset) Instance(name string, data interface{}) render.Render {
	return &render.HTML{
		Template: template.Must(ts.Locate(name)),
		Name:     "base",
		Data:     data,
	}
}

var styles = styleset{
	style{
		[]string{
			"https://fonts.googleapis.com/css?family=Montserrat:200,400,700",
			"static/css/404-04.css",
		},
		"Oops!",
		"The page cannot be found",
	},
}

func (ss styleset) randomStyle() style {
	n := len(ss)
	if n == 0 {
		return style{}
	}

	idx := rand.Intn(n)
	return ss[idx]
}

func error501(c *gin.Context) {
	c.AbortWithError(501, errors.New("not implemented"))
}

func appendSlash(c *gin.Context) {
	path := c.Request.URL.Path + "/"
	c.Redirect(301, path)
}
