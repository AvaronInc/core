import React, {StrictMode, useState, useRef} from 'react'
import Frame from '../frame'
import Map from '../map'
import {ctoa, atoc} from '../coordinates'
import {size} from '../util'
import ReactDOM from 'react-dom/client';

const peers = {
	"123421joij1i32j1923jf": {
		name: "bobby",
		endpoint: "192.168.1.1:58120",
		sent: 500000,
		received: 200,
		latestHandshake: 25,
		location: [-118.2436849, 34.0522342],
	},
	"asdafa23421joij1i32j1923jf": {
		name: "boy",
		endpoint: "192.168.1.1:58120",
		sent: 500000,
		received: 200,
		latestHandshake: 25,
		location: [-110.2436849, 34.0522342],
	},
	"asdafa23421joij1i32je923jf": {
		name: "bobby",
		endpoint: "192.168.1.1:58120",
		sent: 500000,
		received: 200,
		latestHandshake: 25,
		location: [-110.2436849, 34.0522342],
	},
}

const Peers = ({peers, selected, setSelected}) => {
	const rows = new Array(peers.length)

	let i = 0
	for (const key in peers) {
		const peer = peers[key]

		rows[i++] = (
			<tr
				key={key}
				class={(key in selected) ? "table-active" : ""}
				onClick={setSelected.bind(null, {[key]: null})}
			>
				<td>{key}</td>
				<td>{peer.name}</td>
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
	for (key of locations[coords]) {
		m[key] = null
	}
	setSelected(m)
}

const Locations = ({peers, locations, selected, setSelected}) => {
	const rows = new Array(size(locations))
	let i = 0
	console.log(locations)
	const matching = {}
	for (const key in selected) {
		matching[ctoa(peers[key].location)] = null
	}


	for (const c in locations) {
		const keys = locations[c]
		const [longitude, latitude] = atoc(c)
		rows[i++] = (
			<tr
				class={(c in matching) ? "table-active" : ""}
				onClick={setSelectedLocation.bind(null, setSelected, locations, c)}
				key={c}
			>
				<td>{longitude}</td>
				<td>{latitude}</td>
				<td></td>
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
	console.log("selected", selected)

	const locations = {}
	for (const key in peers) {
		const peer = peers[key]
		const location = ctoa(peer.location) 
		if (location in locations) {
			locations[location].push(key)
		} else {
			locations[location] = [key]
			console.log(peer)
		}
	}


	return (
		<Frame>
			<div class="card text-bg-dark">
					<Map as="section" map={map} bounds={bounds} locations={locations} peers={peers} selected={selected} setSelected={setSelected} />
			</div>
			<div class="d-flex flex-row mt-2 w-100">
				<div class="card text-bg-dark flex-fill overflow-x-auto me-2 ">
					<div class="card-header">
						Locations
					</div>
					<div class="card-body">
						<Locations peers={peers} selected={selected} setSelected={setSelected} locations={locations}/>
					</div>
				</div>
				<div class="card text-bg-dark flex-fill overflow-x-auto">
					<div class="card-header">
						Peers
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


