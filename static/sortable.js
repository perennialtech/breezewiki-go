const collator = new Intl.Collator(undefined, {numeric: true, sensitivity: "base"})

for (const table of document.querySelectorAll("table.sortable")) {
	const header = table.tHead?.rows[0] ?? table.rows[0]
	const body = table.tBodies[0]
	if (!header || !body) continue

	for (const [index, cell] of Array.from(header.cells).entries()) {
		if (cell.tagName !== "TH" || cell.colSpan !== 1) continue
		let ascending = true
		cell.tabIndex = 0
		cell.setAttribute("role", "button")
		cell.setAttribute("aria-sort", "none")
		const sort = () => {
			const rows = Array.from(body.rows).filter(row => row !== header)
			rows.sort((a, b) => {
				const left = a.cells[index]?.textContent.trim() ?? ""
				const right = b.cells[index]?.textContent.trim() ?? ""
				return collator.compare(left, right) * (ascending ? 1 : -1)
			})
			for (const sibling of header.cells) sibling.setAttribute("aria-sort", "none")
			cell.setAttribute("aria-sort", ascending ? "ascending" : "descending")
			for (const row of rows) body.appendChild(row)
			ascending = !ascending
		}
		cell.addEventListener("click", sort)
		cell.addEventListener("keydown", event => {
			if (event.key === "Enter" || event.key === " ") {
				event.preventDefault()
				sort()
			}
		})
	}
}
