import { useCallback, useEffect, useRef } from 'react';
import React from 'react';

import { default as OLMap }     from 'ol/Map.js';
import Feature                  from 'ol/Feature.js';
import View                     from 'ol/View.js';
import { boundingExtent }       from 'ol/extent';
import { fromLonLat, toLonLat } from 'ol/proj.js';
import Circle                   from 'ol/geom/Circle.js';
import Fill                     from 'ol/style/Fill.js';
import OSM                      from 'ol/source/OSM.js';
import Stroke                   from 'ol/style/Stroke.js';
import Style                    from 'ol/style/Style.js';
import TileLayer                from 'ol/layer/Tile.js';
import VectorLayer              from 'ol/layer/Vector.js';
import VectorSource             from 'ol/source/Vector.js';
import {defaults as Interactions} from 'ol/interaction/defaults.js';

import { size }                   from './util';
import { ctoa, atoc }             from './coordinates';

const yellow = new Style({
	fill: new Fill({
		color: 'rgba(255,255,0,0.4)',
	}),
})

const blue = new Style({
	fill: new Fill({
		color: 'rgba(255,255,255,0.4)',
	}),
	stroke: new Stroke({
		color: '#3399CC',
		width: 1.25
	}),
})

function preventDefault(e) {
	e.preventDefault()
}

function extentFromBounds(bounds) {
	if (!bounds || bounds.length < 4) {
		return [-180, -90, 180, 90]
	}
	const extent = new Array(4)
	// work-around for query of "russia"
	extent[0] = bounds[0]
	extent[1] = bounds[1]
	if (bounds[0] > bounds[2]) {
		if (bounds[2] > 0) {
			extent[2] = 360.0 - bounds[2]
		} else {
			extent[2] = 360.0 + bounds[2]
		}
	} else {
		extent[2] = bounds[2]
	}
	if (bounds[1] > bounds[3]) {
		if (bounds[3] > 0) {
			extent[3] = 180.0 - bounds[3]
		} else {
			extent[3] = 180.0 + bounds[3]
		}
	} else {
		extent[3] = bounds[3]
	}
	return extent
}

function FindPeer(nodes, key) {
	for (const k in nodes) {
		for (const j in nodes[k].tunnels) {
			if (key in nodes[k].tunnels[j].peers) {
				return nodes[k].tunnels[j].peers[key]
			}
		}
	}
	return null
}

export default function Map({ as: Tag = 'div', map, bounds, locations, nodes, selected, setSelected, ...rest }) {
	const container = useRef(null);
	const circles   = useRef(new VectorSource({
		features: [],
	}));

	useEffect(() => {
		if (map.current) {
			return
		}

		map.current = new OLMap({
			layers: [
				new TileLayer({
					source: new OSM(),
				}),
				new VectorLayer({
					source: circles.current,
				})
			],
			view: new View({
				center: [0, 0],
				zoom: 0,
			}),
			interactions: Interactions({
				pinchRotate: false
			}),
			controls: [],
		})

	}, [map])

	useEffect(() => {
		circles.current.clear()

		if (locations === null || typeof locations !== "object") {
			return
		}

		const features = new Array(size(locations))

		let i = 0
		for (let json in locations) {
			const coords = atoc(json)
			const transform = fromLonLat(coords)
			features[i++] = new Feature({
				geometry: new Circle(transform, 100000)
			})
		}

		circles.current.addFeatures(features)
	}, [locations])

	const handleClick = useCallback(({ pixel }) => {
		const coord = map.current.getCoordinateFromPixel(pixel)
		const circle = circles.current.getClosestFeatureToCoordinate(coord)
		if (!circle) {
			return
		}
		const center = circle.getGeometry().getCenter()

		const m = {}
		for (key of locations[ctoa(toLonLat(center))].nodes) {
			m[key] = null
		}
		setSelected(m)
		}, [setSelected, locations])

	useEffect(() => {
		map.current.on('click', handleClick)
		return map.current.un.bind(map.current, 'click', handleClick)
	}, [map, handleClick])

	useEffect(() => {
		if (selected === "" || selected === null) {
			return
		}

		const m = {}
		for (const key in selected) {
			let location, coords
			if (key in nodes) {
				const {longitude, latitude} = nodes[key].location
				coords = [longitude, latitude]
				location = ctoa(coords)
			} else if ((node = FindPeer(nodes, key))) {
				for (location in locations) {
					if (locations[location].nodes.includes(key)) {
						coords = atoc(location)
						break
					}
				}
				if (coords == null) {
					continue
				}
				// ok
			} else {
				continue
			}

			if (location in m) {
				continue
			}
			m[location] = null
			const features = circles.current.getFeaturesAtCoordinate(fromLonLat(coords))
			for (const feature of features) {
				console.log("setting yellow", coords)
				feature.setStyle(yellow)
			}
			
		}

		return () => {
			for (const location in m) {
				const coords = atoc(location)
				for (const feature of circles.current.getFeaturesAtCoordinate(fromLonLat(coords))) {
					feature.setStyle(blue)
				}
			}
		}
	}, [selected]);

	useEffect(() => {
		const element = container.current
		element.addEventListener('selectstart', preventDefault);
		element.addEventListener('touchstart', preventDefault);
		map.current.setTarget(element)
		return () => {
			map.current.setTarget(null)
			element.removeEventListener('touchstart', preventDefault);
			element.removeEventListener('selectstart', preventDefault);
		}
	}, [container]);

	return ( <Tag {...rest} ref={container} /> );
}
