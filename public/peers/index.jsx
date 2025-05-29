import React, {StrictMode, useState, useEffect, useRef} from 'react'
import Frame from '../frame'
import {size} from '../util'
import ReactDOM from 'react-dom/client';

const Peers = () => {
	const [peers,       setPeers] = useState({})

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
			<div class="d-flex flex-row w-100">
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
