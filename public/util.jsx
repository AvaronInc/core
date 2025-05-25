export function size(obj) {
	let n = 0
	for (let k in obj) {
		n++
	}
	return n
}
