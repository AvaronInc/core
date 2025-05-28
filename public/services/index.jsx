import React, {StrictMode, useState, useEffect, useRef} from 'react'
import Frame from '../frame'
import {size} from '../util'
import ReactDOM from 'react-dom/client';

function style(active, sub) {
	let s = ["secondary", "secondary"]
	switch (active) {
	case "inactive": return s
	case "failed": s[0] = "warning"; break
	default:       s[0] = "info";    break
	}

	switch (sub) {
	case "exited":  s[1] = "secondary"; break;
	case "dead":    s[1] = "warning";   break;
	case "failed":  s[1] = "danger";    break;
	case "running": s[1] = "success";   break;
	default:        s[1] = "danger";    break;
	}

	return s
}

const List = () => {
	const [services, setServices] = useState(null)
	const [focus, setFocus] = useState(null)

	useEffect(() => {
		fetch("/api/services")
			.then(r => r.json())
			.then(services => (console.log("got services", services), services))
			.then(setServices)
	}, [])

	const keys = services ? Object.keys(services) : []
	keys.sort((a, b) => {
		const as = services[a].ActiveState, bs = services[b].ActiveState
		if (as === bs || (as === "active" && bs === "failed") || (as === "failed" && bs === "active")) {
			// ok
		} else if (services[a].ActiveState == "inactive") {
			return 1
		} else {
			return -1
		}

		if (a === b) {
			return 0
		} else if (a < b) {
			return -1
		} else {
			return 1
		}

	})
	
	const rows = new Array(keys.length)

	let i = 0
	for (const key of keys) {
		const s = services[key]
		const styles = style(s.ActiveState, s.SubState)
		rows[i++] = (
			<tr
				key={key}
				class={(key === focus) ? "table-active" : ""}
				onClick={setFocus.bind(null, key)}
			>
				<td>{s.Name.replace(/\.service$/, "")}</td>
				<td>
					<strong class={"m-1 py-1 px-2 rounded-pill bg-"+styles[0]}>
						{s.ActiveState}
					</strong>
				</td>
				<td>
					<strong class={"m-1 py-1 px-2 rounded-pill bg-"+styles[1]}>
						{s.SubState}
					</strong>
				</td>
				{focus ? null : (
					<td>
						<span class={"m-1 py-1 px-2"}>
							{s.Description}
						</span>
					</td>
				)}
			</tr>
		)
	}

	return (
		<Frame>
			<div class="d-flex flex-row w-100">
				{services ? (
					<div class="card text-bg-dark overflow-x-auto me-2 ">
						<div class="card-body">
							<table class="table table-dark">
								<tbody>
									{rows}
								</tbody>
							</table>
						</div>
					</div>
				) : null}
				{focus ? (
					<div class="card text-bg-dark flex-fill overflow-x-auto me-2 ">
						<div class="card-header">
							{focus}
						</div>
						<div class="card-body">
							<p>
								{services[focus].Description}
							</p>
						</div>
					</div>
				) : null}
			</div>
		</Frame>
	)
}

const root = ReactDOM.createRoot(document.getElementById('root'));
root.render(
	<StrictMode>
		<List />
	</StrictMode>
);
