import React, {StrictMode, useState, useEffect, useRef} from 'react'
import Frame from '../frame'
import {size} from '../util'
import ReactDOM from 'react-dom/client';

const Peers = () => {
	const [peers,       setPeers] = useState({})
	const [qr,      setQR] = useState(false)
	const svg = useRef(null)


	useEffect(() => {
		if (!qr || !svg.current) {
			console.log("svg.current", svg.current)
			return
		}

		const promise = fetch("/api/wireguard", { method: "POST" })
			.then(r => r.text())
			.then(t => (console.log("setting inner html"), (svg.current.innerHTML = t)))

		return () => {
			promise.finally(() => svg.current.innerHTML = "")
		}
	}, [qr])
	useEffect(() => {
		fetch("/api/wireguard")
			.then(r => r.json())
			.then(interfaces => {
				(console.log("got interfaces", interfaces), interfaces)
				const peers = {}
				for (k in interfaces) {
					for (l in interfaces[k].peers) {
						const peer = interfaces[k].peers[l]
						peer.interface = interfaces[k].name
						peers[l] = peer
					}
				}
				return peers
			})
			.then(setPeers)
	}, [])

	const rows = new Array(size(peers))
	let i = 0

	for (const key in peers) {
		const peer = peers[key]

		rows[i++] = (
			<tr
				key={key}
			>
				<td class="py-3"  title={key}><tt>{key}</tt></td>
				<td class="py-3" >{peer.interface}</td>
				<td class="py-3" >{peer.endpoint}</td>
				<td class="py-3" >{peer.sent}</td>
				<td class="py-3" >{peer.received}</td>
				<td class="py-3" >{peer.latestHandshake}</td>
				<td>
					<button
						disabled={peer.interface !== "avaron"}
						type="button"
						class="btn btn-danger"
					>
						Delete
					</button>
				</td>

			</tr>
		)
	}

	return (
		<Frame>
			<div class="d-flex flex-column w-100">
				<div class="card text-bg-dark flex-fill overflow-x-auto">
					<div class="card-header">
						Wireguard Peers
					</div>
					<div class="card-body">
						<table class="table table-dark">
							<thead>
								<tr>
									<th scope="col">Key</th>
									<th scope="col">Interface</th>
									<th scope="col">Endpoint</th>
									<th scope="col">Sent</th>
									<th scope="col">Received</th>
									<th scope="col">Last Seen</th>
									<th scope="col">Delete</th>
								</tr>
							</thead>
							<tbody>
								{rows}
							</tbody>
						</table>
						<div class="w-100"  >
							<div class="mb-1 flex-2" ref={svg}>
							</div>
							<button
								type="button"
								class={"btn " + (qr ? "btn-primary" : "btn-success")}
								onClick={() => setQR(qr => !qr)}
							>
								{qr ? "Hide" : "Add"}
							</button>
						</div>
					</div>
				</div>
			</div>
		</Frame>
	)
}

const root = ReactDOM.createRoot(document.getElementById('root'));
root.render(
	<StrictMode>
		<Peers />
	</StrictMode>
);
