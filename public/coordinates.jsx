const m = Math.pow(10, 12)
function formatFloat(f) {
       return (Math.round(f * m)/m).toString()
}

export function ctoa(c) {
	const m = Math.pow(10, 12)
	return formatFloat(c[0]) + "," + formatFloat(c[1])
}

export function atoc(a) {
	return [
		parseFloat(a),
		parseFloat(a.slice(a.indexOf(',')+1))
	]
}

export const regex = /[+-]?[0-9]{0,3}\.[0-9]{0,20},[+-]?[0-9]{0,3}\.[0-9]{0,20}/;

