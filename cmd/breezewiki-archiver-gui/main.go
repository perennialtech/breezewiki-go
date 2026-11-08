// Command breezewiki-archiver-gui opens a local desktop browser control panel.
// It requires no GTK/Qt libraries and works with headless Go builds.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/perennialtech/breezewiki-go/internal/archive"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	storage := flag.String("storage", "storage", "directory containing archiver.db and archive/")
	listen := flag.String("listen", "127.0.0.1:0", "local control panel address")
	noBrowser := flag.Bool("no-browser", false, "print the URL without opening a browser")
	flag.Parse()
	a, err := archive.Open(*storage)
	if err != nil {
		return err
	}
	defer a.Close()
	queue, err := archive.NewQueue(a)
	if err != nil {
		return err
	}
	defer queue.Close()
	mux := handler(queue)
	listener, err := net.Listen("tcp", *listen)
	if err != nil {
		return err
	}
	server := &http.Server{Handler: mux, ReadHeaderTimeout: 10 * time.Second}
	url := "http://" + listener.Addr().String() + "/"
	fmt.Println("Fandom Archiver:", url)
	if !*noBrowser {
		if err := openBrowser(url); err != nil {
			log.Printf("Unable to open a browser: %v (open %s manually)", err, url)
		}
	}
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-stop
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		server.Shutdown(ctx)
	}()
	err = server.Serve(listener)
	if err == http.ErrServerClosed {
		return nil
	}
	return err
}

func openBrowser(url string) error {
	switch runtime.GOOS {
	case "darwin":
		return exec.Command("open", url).Start()
	case "windows":
		return exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
	default:
		// Chromium's app mode gives the control panel its own desktop window
		// without introducing a native GUI toolkit build dependency.
		for _, candidate := range []string{"chromium", "chromium-browser", "google-chrome"} {
			if exe, err := exec.LookPath(candidate); err == nil {
				return exec.Command(exe, "--app="+url).Start()
			}
		}
		return exec.Command("xdg-open", url).Start()
	}
}

func handler(queue *archive.Queue) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		fmt.Fprint(w, dashboard)
	})
	mux.HandleFunc("GET /state", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		json.NewEncoder(w).Encode(queue.Snapshot())
	})
	mux.HandleFunc("POST /action", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Origin") != "" && r.Header.Get("Origin") != "http://"+r.Host {
			http.Error(w, "invalid origin", http.StatusForbidden)
			return
		}
		if err := r.ParseForm(); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		wiki := strings.TrimSpace(r.PostForm.Get("wiki"))
		var err error
		switch r.PostForm.Get("action") {
		case "add":
			err = queue.Add(wiki)
		case "resume":
			err = queue.Resume(wiki)
		case "pause":
			err = queue.Pause(wiki)
		case "reset":
			err = queue.Reset(wiki, false)
		case "reset-pages":
			err = queue.Reset(wiki, true)
		default:
			http.Error(w, "unknown action", http.StatusBadRequest)
			return
		}
		if err != nil {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		if r.Header.Get("Accept") == "application/json" {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		http.Redirect(w, r, "/", http.StatusSeeOther)
	})
	return mux
}

const dashboard = `<!doctype html>
<html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1">
<title>Fandom Archiver</title>
<style>
body{font:15px system-ui,sans-serif;max-width:960px;margin:2rem auto;padding:0 1rem;color:#222;background:#fafafa}
h1{font-size:1.5rem;font-weight:600}form.add{display:flex;align-items:center;gap:.5rem;flex-wrap:wrap;margin-bottom:1.25rem}
input{font:inherit;padding:.45rem;border:1px solid #999;border-radius:3px}button{font:inherit;padding:.35rem .65rem;cursor:pointer;border:1px solid #999;background:#fff;border-radius:3px}
button:hover{background:#eee}button:focus-visible,input:focus-visible{outline:2px solid #1477ba}
table{border-collapse:collapse;width:100%}th{text-align:left;font-weight:600}td,th{padding:.65rem .4rem;border-bottom:1px solid #ddd;vertical-align:middle}
td.actions{white-space:nowrap}td.actions button{margin-right:.25rem}.bar{display:flex;gap:2px;height:12px;min-width:120px;background:#ddd}
.bar span{background:#2a9a82}.dim{color:#666;font-size:.88rem}.error{color:#b00;overflow-wrap:anywhere}
#message{min-height:1.5rem;color:#b00}@media(max-width:650px){.actions button{margin-bottom:.3rem}td{font-size:.9rem} .bar{min-width:60px}}
</style></head><body>
<h1>Fandom Archiver</h1>
<form class="add" id="add"><span>https://</span><input name="wiki" id="wiki" aria-label="Wiki name" placeholder="wikiname" required pattern="[a-zA-Z0-9-]{1,50}"><span>.fandom.com</span><button type="submit">Download Wiki</button></form>
<div id="message" role="alert"></div>
<table><thead><tr><th>Wiki</th><th>Status</th><th>Progress</th><th>Actions</th></tr></thead><tbody id="items"></tbody></table>
<script>
const items = document.getElementById('items'); const message = document.getElementById('message');
async function action(which,wiki){
  const res = await fetch('/action',{method:'POST',headers:{'Content-Type':'application/x-www-form-urlencoded','Accept':'application/json'},body:new URLSearchParams({action:which,wiki:wiki})});
  message.textContent = res.ok ? '' : await res.text(); await refresh();
}
function text(parent,tag,value,cls){ const e=document.createElement(tag); e.textContent=value;if(cls)e.className=cls;parent.appendChild(e);return e; }
function button(parent,title,which,wiki){ const b=text(parent,'button',title);b.type='button';b.onclick=()=>action(which,wiki); }
async function refresh(){
  const res=await fetch('/state',{cache:'no-store'});if(!res.ok)return;
  const data=await res.json();items.replaceChildren();
  for(const item of data){
    const tr=document.createElement('tr');items.appendChild(tr);
    text(tr,'td',item.wiki);
    const status=document.createElement('td');tr.appendChild(status);
    text(status,'div',item.state);
    if(item.error)text(status,'div',item.error,'error');
    const progress=document.createElement('td');tr.appendChild(progress);
    const stage=Math.max(0,Math.min(4,item.stage));
    const pieces=[5,88,2,5];const bar=text(progress,'div','','bar');
    for(let i=0;i<4;i++){
      const segment=document.createElement('span');segment.style.width=pieces[i]+'%';
      const fraction=i<stage?1:i>stage?0:(item.total>0?Math.max(0,Math.min(1,item.progress/item.total)):0);
      segment.style.background='linear-gradient(to right,#2a9a82 '+(fraction*100)+'%,#ddd '+(fraction*100)+'%)';bar.appendChild(segment);
    }
    text(progress,'div',stage===4?'Complete':('Stage '+(stage+1)+'/4 · '+item.progress+'/'+item.total),'dim');
    if(item.name)text(progress,'div',item.name,'dim');
    const actions=document.createElement('td');actions.className='actions';tr.appendChild(actions);
    if(item.state==='running') button(actions,'Pause','pause',item.wiki);
    else {button(actions,'Resume','resume',item.wiki);button(actions,'Reset','reset',item.wiki);button(actions,'Reset pages','reset-pages',item.wiki);}
  }
}
document.getElementById('add').onsubmit=async e=>{e.preventDefault();const input=document.getElementById('wiki');await action('add',input.value.trim());if(!message.textContent)input.value='';};
refresh();setInterval(refresh,1000);
</script></body></html>`
