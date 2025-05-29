import React, {useState, useEffect} from 'react'

const Icons = {
	"Dashboard": (
		<React.Fragment>
			<rect width="7" height="9" x="3" y="3" rx="1"></rect>
			<rect width="7" height="5" x="14" y="3" rx="1"></rect>
			<rect width="7" height="9" x="14" y="12" rx="1"></rect>
			<rect width="7" height="5" x="3" y="16" rx="1"></rect>
		</React.Fragment>
	),
	"AIM": (
		<React.Fragment>
			<path d="M12 5a3 3 0 1 0-5.997.125 4 4 0 0 0-2.526 5.77 4 4 0 0 0 .556 6.588A4 4 0 1 0 12 18Z"></path>
			<path d="M12 5a3 3 0 1 1 5.997.125 4 4 0 0 1 2.526 5.77 4 4 0 0 1-.556 6.588A4 4 0 1 1 12 18Z"></path>
			<path d="M15 13a4.5 4.5 0 0 1-3-4 4.5 4.5 0 0 1-3 4"></path>
			<path d="M17.599 6.5a3 3 0 0 0 .399-1.375"></path>
			<path d="M6.003 5.125A3 3 0 0 0 6.401 6.5"></path>
			<path d="M3.477 10.896a4 4 0 0 1 .585-.396"></path>
			<path d="M19.938 10.5a4 4 0 0 1 .585.396"></path>
			<path d="M6 18a4 4 0 0 1-1.967-.516"></path>
			<path d="M19.967 17.484A4 4 0 0 1 18 18"></path>
		</React.Fragment>
	),
	"Services": (
		<React.Fragment>
			<circle cx="12" cy="12" r="10"></circle>
			<path d="M17 12h.01"></path>
			<path d="M12 12h.01"></path>
			<path d="M7 12h.01"></path>
		</React.Fragment>
	),
	"Topology": (
		<React.Fragment>
			<rect x="16" y="16" width="6" height="6" rx="1"></rect>
			<rect x="2" y="16" width="6" height="6" rx="1"></rect>
			<rect x="9" y="2" width="6" height="6" rx="1"></rect>
			<path d="M5 16v-3a1 1 0 0 1 1-1h12a1 1 0 0 1 1 1v3"></path>
			<path d="M12 12V8"></path>
		</React.Fragment>
	),
	"DNS": (
		<React.Fragment>
			<circle cx="12" cy="12" r="10"></circle>
			<path d="M12 2a14.5 14.5 0 0 0 0 20 14.5 14.5 0 0 0 0-20"></path>
			<path d="M2 12h20"></path>
		</React.Fragment>
	),
	"Addressing": (
		<React.Fragment>
			<path d="M20 10c0 4.993-5.539 10.193-7.399 11.799a1 1 0 0 1-1.202 0C9.539 20.193 4 14.993 4 10a8 8 0 0 1 16 0"></path>
			<circle cx="12" cy="10" r="3"></circle>
		</React.Fragment>
	),
	"Containers": (
		<React.Fragment>
			<path d="M11 21.73a2 2 0 0 0 2 0l7-4A2 2 0 0 0 21 16V8a2 2 0 0 0-1-1.73l-7-4a2 2 0 0 0-2 0l-7 4A2 2 0 0 0 3 8v8a2 2 0 0 0 1 1.73z"></path>
			<path d="M12 22V12"></path>
			<path d="m3.3 7 7.703 4.734a2 2 0 0 0 1.994 0L20.7 7"></path>
			<path d="m7.5 4.27 9 5.15"></path>
		</React.Fragment>
	),
	"Version Control": (
		<React.Fragment>
			<rect width="8" height="4" x="8" y="2" rx="1" ry="1"></rect>
			<path d="M16 4h2a2 2 0 0 1 2 2v14a2 2 0 0 1-2 2H6a2 2 0 0 1-2-2V6a2 2 0 0 1 2-2h2"></path>
			<path d="M12 11h4"></path>
			<path d="M12 16h4"></path>
			<path d="M8 11h.01"></path>
			<path d="M8 16h.01"></path>
		</React.Fragment>
	),	
	"Peers": (
		<React.Fragment>
			<path d="M16 21v-2a4 4 0 0 0-4-4H6a4 4 0 0 0-4 4v2"></path>
			<circle cx="9" cy="7" r="4"></circle>
			<path d="M22 21v-2a4 4 0 0 0-3-3.87"></path>
			<path d="M16 3.13a4 4 0 0 1 0 7.75"></path>
		</React.Fragment>
	),
	"Security": (
		<React.Fragment>
			<path d="M20 13c0 5-3.5 7.5-7.66 8.95a1 1 0 0 1-.67-.01C7.5 20.5 4 18 4 13V6a1 1 0 0 1 1-1c2 0 4.5-1.2 6.24-2.72a1.17 1.17 0 0 1 1.52 0C14.51 3.81 17 5 19 5a1 1 0 0 1 1 1z"></path>
		</React.Fragment>
	),
	"Firewall": (
		<React.Fragment>
			<rect width="18" height="11" x="3" y="11" rx="2" ry="2"></rect>
			<path d="M7 11V7a5 5 0 0 1 10 0v4"></path>
		</React.Fragment>
	),	
	"Logs": (
		<React.Fragment>
			<path d="M15 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V7Z"></path>
			<path d="M14 2v4a2 2 0 0 0 2 2h4"></path>
			<path d="M10 9H8"></path>
			<path d="M16 13H8"></path>
			<path d="M16 17H8"></path>
		</React.Fragment>
	),	
	"Storage": (
		<React.Fragment>
			<path d="m8 2 1.88 1.88"></path>
			<path d="M14.12 3.88 16 2"></path>
			<path d="M9 7.13v-1a3.003 3.003 0 1 1 6 0v1"></path>
			<path d="M12 20c-3.3 0-6-2.7-6-6v-3a4 4 0 0 1 4-4h4a4 4 0 0 1 4 4v3c0 3.3-2.7 6-6 6"></path>
			<path d="M12 20v-9"></path>
			<path d="M6.53 9C4.6 8.8 3 7.1 3 5"></path>
			<path d="M6 13H2"></path>
			<path d="M3 21c0-2.1 1.7-3.9 3.8-4"></path>
			<path d="M20.97 5c0 2.1-1.6 3.8-3.5 4"></path>
			<path d="M22 13h-4"></path>
			<path d="M17.2 17c2.1.1 3.8 1.9 3.8 4"></path>
		</React.Fragment>
	),	
	"Settings": (
		<React.Fragment>
			<path d="M12.22 2h-.44a2 2 0 0 0-2 2v.18a2 2 0 0 1-1 1.73l-.43.25a2 2 0 0 1-2 0l-.15-.08a2 2 0 0 0-2.73.73l-.22.38a2 2 0 0 0 .73 2.73l.15.1a2 2 0 0 1 1 1.72v.51a2 2 0 0 1-1 1.74l-.15.09a2 2 0 0 0-.73 2.73l.22.38a2 2 0 0 0 2.73.73l.15-.08a2 2 0 0 1 2 0l.43.25a2 2 0 0 1 1 1.73V20a2 2 0 0 0 2 2h.44a2 2 0 0 0 2-2v-.18a2 2 0 0 1 1-1.73l.43-.25a2 2 0 0 1 2 0l.15.08a2 2 0 0 0 2.73-.73l.22-.39a2 2 0 0 0-.73-2.73l-.15-.08a2 2 0 0 1-1-1.74v-.5a2 2 0 0 1 1-1.74l.15-.09a2 2 0 0 0 .73-2.73l-.22-.38a2 2 0 0 0-2.73-.73l-.15.08a2 2 0 0 1-2 0l-.43-.25a2 2 0 0 1-1-1.73V4a2 2 0 0 0-2-2z"></path>
			<circle cx="12" cy="12" r="3"></circle>
		</React.Fragment>
	)
}

export default Frame = ({children}) => {
	const [spin, setSpin] = useState(false)

	useEffect(() => {
		if (!spin) {
			return
		}
		const t = setTimeout(() => {
			setSpin(false)
		}, 400)
		return () => clearTimeout(t)

	}, [spin])

	const icons = new Array(Icons.length)
	const path = window.location.pathname.replace(/\/+/g, '')

	let i = 0

	for (const title in Icons) {
		const key = title.toLowerCase().replace(/\s+/g, '-')
		icons[i++] = (
			<li key={key} class="nav-item">
				<a
					class={"ps-3 pe-5 nav-link" + (key === path ? " active" : "")}
					href={"/"+key}
				>
					<svg
						xmlns="http://www.w3.org/2000/svg"
						width="24"
						height="24"
						viewbox="0 0 24 24"
						fill="none"
						stroke="currentcolor"
						stroke-width="1"
						stroke-linecap="round"
						stroke-linejoin="round"
						class="me-3"
					> {Icons[title]} </svg>
					<small>{title}</small>
				</a>
			</li>
		)
	}

	return (
		<React.Fragment>
			<nav
				style={{height: "100vh", scrollbarWidth: "none" }}
				class="navbar-dark bg-dark pt-2 overflow-y-scroll text-nowrap"
			>
				<ul class="navbar-nav h-100">
					{icons}
					<li
						class="nav-item mt-auto ms-3 me-5 mb-3 p-0 nav-link d-flex justify-content-evenly"
						onClick={(e) => {e.preventDefault(); setSpin(true)}}
					>
						<img
							width="30"
							src="/favicon.png"
							height="30"
							class={"my-auto" + (spin ? " spin" : "")}

						/>
						<small class="text-center" style={{fontSize: ".75em" }} >Avaron Holdings, LLC<br/>© Copyright 2025</small>
					</li>
				</ul>
			</nav>
			<div
				style={{height: "100vh", width: "100%", scrollbarWidth: "none" }}
				class="overflow-y-scroll p-2"
			>
				{children}
			</div>
		</React.Fragment>
	)
}
