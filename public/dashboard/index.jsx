import React, {StrictMode, useState, useEffect, useRef} from 'react'
import Frame from '../frame'
import Map from '../map'
import {ctoa, atoc} from '../coordinates'
import {size} from '../util'
import ReactDOM from 'react-dom/client';

const Peers = ({peers, selected, setSelected}) => {
	const rows = new Array(size(peers))

	let i = 0
	for (const key in peers) {
		const peer = peers[key]

		rows[i++] = (
			<tr
				key={key}
				class={(key in selected) ? "table-active" : ""}
				onClick={setSelected.bind(null, {[key]: null})}
			>
				<td title={key}>{key.slice(0, 8)}...</td>
				<td>{peer.name ? peer.name : "-"}</td>
				<td>{peer.endpoint}</td>
				<td>{peer.sent}</td>
				<td>{peer.received}</td>
				<td>{peer.latestHandshake}</td>
			</tr>
		)
	}

	return (
		<table class="table table-dark">
			<thead>
				<tr>
					<th scope="col">Key</th>
					<th scope="col">Name</th>
					<th scope="col">Endpoint</th>
					<th scope="col">Sent</th>
					<th scope="col">Received</th>
					<th scope="col">Last Seen</th>
				</tr>
			</thead>
			<tbody>
				{rows}
			</tbody>
		</table>
	)
}

function setSelectedLocation(setSelected, locations, coords) {
	const m = {}
	for (key of locations[coords].nodes) {
		m[key] = null
	}
	setSelected(m)
}

const Locations = ({nodes, locations, selected, setSelected}) => {
	const rows = new Array(size(locations))
	let i = 0
	const matching = {}
	for (const key in selected) {
		if (!(key in nodes)) {
			continue
		}
		const {longitude, latitude} = nodes[key].location
		matching[ctoa([longitude, latitude])] = null
	}


	for (const c in locations) {
		const keys = locations[c].nodes
		const [longitude, latitude] = atoc(c)
		rows[i++] = (
			<tr
				class={(c in matching) ? "table-active" : ""}
				onClick={setSelectedLocation.bind(null, setSelected, locations, c)}
				key={c}
			>
				<td>{longitude}</td>
				<td>{latitude}</td>
				<td>{locations[c].city}</td>
				<td>{keys.length}</td>
			</tr>
		)
	}

	return (
		<table class="table table-dark">
			<thead>
				<tr>
					<th scope="col">Longitude</th>
					<th scope="col">Latitude</th>
					<th scope="col">City</th>
					<th scope="col">Count</th>
				</tr>
			</thead>
			<tbody>
				{rows}
			</tbody>
		</table>
	)
}


const Dashboard = () => {
	const map = useRef()
	const [items,       setItems] = useState(null)
	const [bounds,     setBounds] = useState(null)
	const [selected, setSelected] = useState({})
	const [nodes,       setNodes] = useState({})

	useEffect(() => {
		fetch("/api/nodes")
			.then(r => r.json())
			.then(nodes => (console.log("got nodes", nodes), nodes))
			.then(setNodes)
	}, [])

	const peers = {}
	for (k in nodes) {
		for (j in nodes[k].tunnels) {
			for (l in nodes[k].tunnels[j].peers) {
				peers[l] = nodes[k].tunnels[j].peers[l]
			}
		}
	}

	const locations = {}
	for (const key in nodes) {
		const node = nodes[key]
		const {longitude, latitude, city} = node.location
		const location = ctoa([longitude, latitude])
		if (location in locations) {
			locations[location].nodes.push(key)
		} else {
			locations[location] = {
				nodes: [key],
				city: city,
			}
		}
	}


	return (
		<Frame>
			<div class="card text-bg-dark">
					<Map as="section" map={map} bounds={bounds} locations={locations} nodes={nodes} selected={selected} setSelected={setSelected} /> </div>
			<div class="d-flex flex-row mt-2 w-100">
				<div class="card text-bg-dark flex-fill overflow-x-auto me-2 ">
					<div class="card-header">
						Locations
					</div>
					<div class="card-body">
						<Locations nodes={nodes} selected={selected} setSelected={setSelected} locations={locations}/>
					</div>
				</div>
				<div class="card text-bg-dark flex-fill overflow-x-auto">
					<div class="card-header">
						Wireguard Peers
					</div>
					<div class="card-body">
						<Peers selected={selected} setSelected={setSelected} locations={locations} peers={peers} />
					</div>
				</div>
			</div>
		</Frame>
	)
}

const root = ReactDOM.createRoot(document.getElementById('root'));
root.render(
	<StrictMode>
		<Dashboard />
	</StrictMode>
);


