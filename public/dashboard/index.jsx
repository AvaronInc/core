import React, {StrictMode} from 'react'
import Frame from '../frame'
import ReactDOM from 'react-dom/client';

const peers = {
	"123421joij1i32j1923jf": {
		name: "bobby",
		endpoint: "192.168.1.1:58120",
		sent: 500000,
		received: 200,
		latestHandshake: 25,
	},
}



const Peers = () => {
	const rows = new Array(peers.length)
	let i = 0

	for (const key in peers) {
		const peer = peers[key]
		rows[i] = (
			<tr key={key}>
				<td>{key}</td>
				<td>{peer.name}</td>
				<td>{peer.endpoint}</td>
				<td>{peer.sent}</td>
				<td>{peer.received}</td>
				<td>{peer.latestHandshake}</td>
			</tr>
		)
		i++
	}

	return (
		<Frame>
			<div class="container-md">
				<table class="table table-dark">
					<thead>
						<tr>
							<th scope="col">Key</th>
							<th scope="col">Hostname</th>
							<th scope="col">Endpoint</th>
							<th scope="col">Sent</th>
							<th scope="col">Received</th>
							<th scope="col">Last Handshake</th>
						</tr>
					</thead>
					<tbody>
						{rows}
					</tbody>
				</table>
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


