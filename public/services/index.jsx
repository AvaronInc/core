import React, {StrictMode, useState, useEffect, useRef} from 'react'
import Frame from '../frame'
import {size} from '../util'
import ReactDOM from 'react-dom/client';

function style(active, sub) {
	let s = ["secondary", "secondary"]
	switch (active) {
	case "inactive": return s
	case "failed":                   break
	default:       s[0] = "primary"; break
	}

	switch (sub) {
	case "exited":                      break;
	case "dead":    s[1] = "warning";   break;
	case "failed":  s[1] = "danger";    break;
	case "running": s[1] = "success";   break;
	default:        s[1] = "danger";    break;
	}

	return s
}


const Context = ({children}) => (
	<svg
		xmlns="http://www.w3.org/2000/svg"
		width="24"
		height="24"
		viewBox="0 0 24 24"
		fill="none"
		stroke="currentColor"
		stroke-width="2"
		stroke-linecap="round"
		stroke-linejoin="round"
		class="lucide lucide-play h-4 w-4"
		style={{ width: ".75rem", height: ".75rem"}}
	>
		{children}
	</svg>
)

const Start = (
	<Context>
		<polygon points="6 3 20 12 6 21 6 3"></polygon>
	</Context>
)

const Stop = (
	<Context>
		<rect width="18" height="18" x="3" y="3" rx="2"></rect>
	</Context>
)

const Restart = (
	<Context>
		<path d="M3 12a9 9 0 1 0 9-9 9.75 9.75 0 0 0-6.74 2.74L3 8"></path><path d="M3 3v5h5"></path>
	</Context>
)



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
				<td
					class="py-3"
				>
					{s.Name.replace(/\.service$/, "")}
				</td>
				<td class="text-center"  >
					<p class={"m-1 py-1 px-2 rounded bg-"+styles[0]}>
						{s.ActiveState}
					</p>
				</td>
				<td class="text-center">
					<p class={"m-1 py-1 px-2 rounded text-center bg-"+styles[1]}>
						{s.SubState}
					</p>
				</td>
				{focus ? null : (
					<td class="py-3">
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
			<div class="d-flex flex-row w-100 ">
				{services ? (
					<div
						style={{scrollbarWidth: "none", maxHeight: "100vh"}}
						class="card w-100 text-bg-dark overflow-x-auto overflow-auto"
					>
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
					<div class="card w-100 text-bg-dark overflow-x-auto ms-2  me-1 flex-grow">
						<div class="card-header d-flex flex-row">
							{focus}
							<div class="ms-auto">
								<div class="btn-group" role="group" aria-label="Basic example">
									<button
										disabled={services[focus].SubState === "running"}
										type="button"
										class="p-2 btn btn-primary"
									>
										{Start}
									</button>
									<button
										disabled={services[focus].SubState !== "running"}
										type="button"
										class="p-2 btn btn-primary"
									>
										{Stop}
									</button>
									<button
										disabled={services[focus].ActiveState === "inactive"}
										type="button"
										class="p-2 btn btn-primary"
									>
										{Restart}
									</button>
								</div>
								<button
									type="button"
									class="p-2 btn btn-primary ms-1"
									onClick={setFocus.bind(null, null)}
								>
									x
								</button>
							</div>
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
