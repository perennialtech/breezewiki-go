const collator = new Intl.Collator(undefined, {numeric: true, sensitivity: "base"})
const numeric = /^[+-]?(?:\d{1,3}(?:,\d{3})+|\d+)(?:\.\d+)?$/

function columnValue(row, column) {
	let position = 0
	for (const cell of row.cells) {
		position += cell.colSpan
		if (column < position) return (cell.getAttribute("data-sort-value") ?? cell.textContent).trim()
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
		const body = table.tBodies[0]
		if (!body) continue

		const headers = table.tHead ? Array.from(table.tHead.rows) : []
		if (!table.tHead) {
			for (const row of body.rows) {
				if (!row.cells.length || !Array.from(row.cells).every(cell => cell.tagName === "TH" && cell.scope !== "row")) break
				headers.push(row)
			}
		}
		if (!headers.length) continue
		table.dataset.bwSortable = "1"

		const occupied = []
		const leafHeaders = []
		for (const [rowIndex, row] of headers.entries()) {
			let column = 0
			for (const cell of row.cells) {
				while ((occupied[column] ?? 0) > rowIndex) column++
				const index = column
				const until = cell.rowSpan === 0 ? headers.length : rowIndex + cell.rowSpan
				for (let i = 0; i < cell.colSpan; i++) occupied[column++] = until
				if (cell.tagName !== "TH" || cell.colSpan !== 1 || cell.classList.contains("unsortable")) continue
				leafHeaders.push({cell, index})
			}
		}

		for (const {cell, index} of leafHeaders) {
			let ascending = true
			cell.tabIndex = 0
			cell.setAttribute("aria-sort", "none")
			const sort = () => {
				const data = Array.from(body.rows).filter(row => !headers.includes(row))
				const top = data.filter(row => row.classList.contains("sorttop"))
				const bottom = data.filter(row => row.classList.contains("sortbottom"))
				const sortable = data.filter(row => !row.classList.contains("sorttop") && !row.classList.contains("sortbottom"))
				const groups = []
				for (let start = 0; start < sortable.length;) {
					let end = start + 1
					for (let i = start; i < end; i++) {
						for (const item of sortable[i].cells) {
							end = Math.min(sortable.length, Math.max(end, item.rowSpan === 0 ? sortable.length : i + item.rowSpan))
						}
					}
					groups.push(sortable.slice(start, end))
					start = end
				}
				groups.sort((a, b) => compareValues(columnValue(a[0], index), columnValue(b[0], index)) * (ascending ? 1 : -1))
				for (const header of leafHeaders) header.cell.setAttribute("aria-sort", "none")
				cell.setAttribute("aria-sort", ascending ? "ascending" : "descending")
				for (const row of [...(table.tHead ? [] : headers), ...top, ...groups.flat(), ...bottom]) body.appendChild(row)
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
