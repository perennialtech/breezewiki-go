import {h, htm, render, signal, computed, effect} from "preact"
const html = htm.bind(h)

// *** Status

const loaded = signal(false)
export {loaded}

// *** Loading indicator

render(html`Loading, please wait...`, document.getElementById("loading"))

// *** Progress bar

const progress = signal(null)

const progressBar = document.getElementById("progress-bar")
while (progressBar.childNodes[0] !== undefined) progressBar.childNodes[0].remove() // clear out loading indicators

render(html`<progress value="${progress}" max="1"></progress>`, progressBar)

// *** Incoming data processing

// Handle case where data is immediately available
cont()

// Handle case where data may become available in the future
window.proxy = new Proxy(jsonpData, {
	get(obj, prop) {
		return value => {
			obj[prop] = value
			cont()
		}
	}
})

// *** Data upload and download

async function cont() {
	// Check for errors
	for (const v of Object.values(jsonpData)) {
		if (v.error) return error(v)
	}

	// Check for completion
	const dependencies = [...document.querySelectorAll("[data-jsonp-var]")].map(e => e.getAttribute("data-jsonp-var"))
	for (const d of dependencies) {
		if (!(d in jsonpData)) return
	}

	const xhr = new XMLHttpRequest();

	const uploadFraction = 0.7

	const pkg = {
		url: location.href,
		init: {
			method: "POST",
			body: JSON.stringify({
				wikiname: BWData.wikiname,
				path: BWData.path,
				...jsonpData
			})
		}
	}

	// Upload progress
	xhr.upload.addEventListener("progress", event => {
		if (event.lengthComputable) {
			progress.value = (event.loaded / event.total) * uploadFraction
		}
	})

	// Download progress
	xhr.addEventListener("progress", event => {
		if (event.lengthComputable) {
			progress.value = (event.loaded / event.total) * (1 - uploadFraction) + uploadFraction
		}
	})

	xhr.addEventListener("load", () => {
		console.log(xhr)
		// check for errors
		if (xhr.status === 500) return fetch(pkg.url, pkg.init).then(res => res.text()).then(error)
		// check for captcha screen
		if (xhr.responseXML.head.querySelector('script[src*="captcha.js"]')) return location.reload()
		// page -> #content
		const imported = document.importNode(xhr.responseXML.getElementById("content"), true)
		document.getElementById("content").replaceWith(imported)
		// page theme
		// Try to make it a bit more accurate. Normally this is done server-side by sending a `theme=x` cookie with the parse request. But that's not possible in jsonp mode, and there's no equivalent URL query string to set the theme.
		// Helps on Minecraft wiki. Might not be complete on some wikis.
		cookieStore.get("theme").then(cookie => {
			if (cookie && cookie.value !== "default") {
				document.body.classList.remove("theme-fandomdesktop-light")
				document.body.classList.remove("theme-fandomdesktop-dark")
				document.body.removeAttribute("data-theme")
				document.body.classList.add(`theme-fandomdesktop-${cookie.value}`)
				document.body.setAttribute("data-theme", cookie.value)
			}
		})
		// <head>
		document.title = xhr.responseXML.title
		document.body.className = xhr.responseXML.body.className
		for (const e of xhr.responseXML.head.children) {
			const alreadyImported = [...document.querySelectorAll("link[href]")].map(e => e.href)
			if (e.tagName === "LINK" && !alreadyImported.includes(e.href)) {
				const imported = document.importNode(e, true)
				document.head.appendChild(imported)
			}
		}
		// scroll
		if (location.hash) {
			document.getElementById(location.hash.slice(1)).scrollIntoView({behavior: "instant"})
		}
		// redirects
		const redirectTo = document.querySelector("#content .redirectMsg a")
		if (redirectTo) {
			redirectTo.click()
		}
		loaded.value = true
	})

	xhr.open(pkg.init.method, pkg.url)
	xhr.responseType = "document"
	xhr.send(pkg.init.body);
}

function error(data) {
	const eContent = document.getElementById("content")
	while (eContent.childNodes[0] !== undefined) eContent.childNodes[0].remove() // clear out loading indicators
	document.title = `Error | BreezeWiki`
	if (typeof data === "string") {
		render(html`<p><strong>BreezeWiki ran into an error on this page.</strong></p><p>Try reloading the page.</p><p>If this keeps happening, you could <a href="mailto:~cadence/breezewiki-discuss@lists.sr.ht">send a public bug report</a>. Please include the following information:</p><pre>URL: ${window.location.href}${"\n"}${data}</pre>`, eContent)
	} else if (data.error.code === "missingtitle") {
		render(html`<p><strong>This page doesn't exist on Fandom.</strong></p><p><small><a href="/${BWData.wikiname}/wiki/Main_Page">Return to the wiki's main page</a></small></p>`, eContent)
	} else {
		render(html`<p>BreezeWiki wasn't able to load this page.</p><p><strong>${data.error.code}: ${data.error.info}</strong></p>`, eContent)
	}
}
