const collator = new Intl.Collator(undefined, {numeric: true, sensitivity: "base"})
const numeric = /^[+-]?(?:\d{1,3}(?:,\d{3})+|\d+)(?:\.\d+)?$/

function columnValue(row, column) {
	let position = 0
	for (const cell of row.cells) {
		position += cell.colSpan
		if (column < position) {
			return (cell.getAttribute("data-sort-value") ?? cell.textContent).trim()
		}
	}
	return ""
}

function compareValues(a, b) {
	if (numeric.test(a) && numeric.test(b)) {
		return Number(a.replaceAll(",", "")) - Number(b.replaceAll(",", ""))
	}
	return collator.compare(a, b)
}

export function initSortableTables() {
for (const table of document.querySelectorAll("table.sortable")) {
    if (table.dataset.bwSortable) continue
    table.dataset.bwSortable = "1"
	const header = table.tHead?.rows[0] ?? table.rows[0]
	const body = table.tBodies[0]
	if (!header || !body) continue

	let column = 0
	for (const cell of header.cells) {
		const index = column
		column += cell.colSpan
		if (cell.tagName !== "TH" || cell.colSpan !== 1 || cell.classList.contains("unsortable")) continue
		let ascending = true
		cell.tabIndex = 0
		cell.setAttribute("aria-sort", "none")
		const sort = () => {
			const rows = Array.from(body.rows).filter(row => row !== header)
			const top = rows.filter(row => row.classList.contains("sorttop"))
			const bottom = rows.filter(row => row.classList.contains("sortbottom"))
			const sortable = rows.filter(row => !row.classList.contains("sorttop") && !row.classList.contains("sortbottom"))
			sortable.sort((a, b) => compareValues(columnValue(a, index), columnValue(b, index)) * (ascending ? 1 : -1))
			for (const sibling of header.cells) {
				if (sibling.hasAttribute("aria-sort")) sibling.setAttribute("aria-sort", "none")
			}
			cell.setAttribute("aria-sort", ascending ? "ascending" : "descending")
			for (const row of [...top, ...sortable, ...bottom]) body.appendChild(row)
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

}

initSortableTables()
