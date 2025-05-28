import React, {StrictMode, useState, useEffect, useRef} from 'react'
import Frame from '../frame'
import {size} from '../util'
import ReactDOM from 'react-dom/client';

function style(s) {
	switch (s) {
	case "exited":
		return "info"
	case "dead":
		return "danger"
	case "running":
		return "success"
	default:
		return "warning"
	}
}

const List = () => {
	const [services, setServices] = useState(null)

	useEffect(() => {
		fetch("/api/services")
			.then(r => r.json())
			.then(services => (console.log("got services", services), services))
			.then(setServices)
	}, [])

	const rows = new Array(size(services))

	let i = 0
	for (const key in services) {
		const s = services[key]

		rows[i++] = (
			<tr
				key={key}
				class={false ? "table-active" : ""}
				onClick={null}
			>
				<td>{s.Name.replace(/\.service$/, "")}</td>
				<td>
					<span class={"m-1 py-1 px-2 text-center rounded-pill bg-"+style(s.SubState)}>
						{s.SubState}
					</span>
				</td>
			</tr>
		)
	}

	return (
		<Frame>
			<div class="d-flex flex-row w-100">
				<div class="card text-bg-dark flex-fill overflow-x-auto me-2 ">
					<div class="card-body">
						<table class="table table-dark">
							<thead>
								<tr>
									<th scope="col">Name</th>
									<th scope="col">State</th>
								</tr>
							</thead>
							<tbody>
								{rows}
							</tbody>
						</table>
					</div>
				</div>
				<div class="card text-bg-dark flex-fill overflow-x-auto me-2 ">
					<div class="card-header">
						List
					</div>
					<div class="card-body">
						<table class="table table-dark">
							<thead>
								<tr>
									<th scope="col">Name</th>
									<th scope="col">State</th>
								</tr>
							</thead>
							<tbody>
								{rows}
							</tbody>
						</table>
					</div>
				</div>
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
