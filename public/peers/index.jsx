import React, {StrictMode, useState, useEffect, useRef, useCallback} from 'react'
import Frame from '../frame'
import {size} from '../util'
import ReactDOM from 'react-dom/client';

const Peers = () => {
	const [peers,       setPeers] = useState({})
	const [qr,      setQR] = useState(null)
	const [fetching,      setFetching] = useState(false)
	const ref = useRef(null)

	const fetchPeers = useCallback(() => {
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
	}, [setPeers])

	const create = useCallback(() => {
		setFetching(true)
		const promise = fetch("/api/wireguard", { method: "POST" })
			.then(r => r.text())
			.then(t => {
				console.log("setting inner html")
				setQR(t)
				fetchPeers()
			})
	}, [setFetching, fetchPeers, setQR])

	useEffect(fetchPeers, [])

	useEffect(() => {
		if (qr) {
			ref.current.innerHTML = qr
			setFetching(false)
			return () => ref.current.innerHTML = ""
		}
	}, [qr, fetchPeers])

	const rows = new Array(size(peers))
	let i = 0

	for (const key in peers) {
		const peer = peers[key]
		const fn = (key) => {
			fetch("/api/wireguard", {method: "DELETE", body: key})
				.then(fetchPeers)
		}

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
						onClick={fn.bind(null, key)}
						type="button"
						class="btn btn-danger"
					>
						Delete
					</button>
				</td>

			</tr>
		)
	}


	let Button
	console.log("fetching", fetching)
	if (fetching) {
		Button = null
	} else if (qr) {
		Button = (
			<button
				type="button"
				class={"ms-auto me-2 btn " + (qr ? "btn-primary" : "btn-success")}
				onClick={() => setQR(null)}
			>
				Hide
			</button>
		)
	} else {
		Button = (
			<button
				type="button"
				class={"ms-auto me-2 btn " + (qr ? "btn-primary" : "btn-success")}
				onClick={create}
			>
				Create
			</button>
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
						<div class="w-100 d-flex"  >
							<div class="mb-1 flex-grow-1" ref={ref}>
							</div>
							{Button}
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
