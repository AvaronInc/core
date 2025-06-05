import React, {StrictMode, useState, useEffect, useRef, useCallback} from 'react'
import Frame from '../frame'
import {size} from '../util'
import ReactDOM from 'react-dom/client';

const Chat = () => {
	const [prompt,         setPrompt] = useState("")
	const [logs,         setLogs] = useState([])
	const input                   = useRef(null)

	const fetchLogs = useCallback(() => {
		fetch("/api/logs")
			.then(r => r.json())
			.then(l => (console.log("logs", l), l))
			.then(setLogs)
	}, [setLogs])

	const submit = useCallback((e) => {
		e.preventDefault()
		if (!input.current || !input.current.value) {
			return
		}

		const xhr = new XMLHttpRequest();
		xhr.open("POST", "/api/completions", true);

		let i = 0, j = 0
		xhr.onreadystatechange = function(e) {
			switch (xhr.readyState) {
			case 3: case 4: break;
			default: return
			}

			for (; i < xhr.responseText.length; i += j+1) {
				const rest = xhr.responseText.length.slice(i)

				j = rest.indexOf('\n')
				if (j == -1) {
					break
				} else if (j == 0) {
					continue
				}

				const token = JSON.parse(rest.slice("data: ".length, j))
				console.log("got token", token.content)
				setPrompt(chat => chat + token.content)
			}
		};

		setPrompt(prompt => {
			prompt = prompt+"[INST]"+input.current.value+"[/INST]"
			xhr.send({
				prompt,
				model: "mixtral.gguf",
				stream: true,
			});
			return prompt
		})

	}, [setPrompt])

	useEffect(fetchLogs, [])

	let i, j;
	const entries = new Array()

	for (i = 0; i < prompt.length; i += j) {
		console.log("i", i, "j", j, "length", prompt.length, prompt)
		let rest = prompt.slice(i)
		j = rest.indexOf("[INST]")

		switch (j) {
		case 0:
			j = rest.indexOf("[/INST]")
			if (j == -1) {
				throw "bad prompt"
			} else if (j > "[INST]".length) {
				entries.push((
					<p key={i}>
						{prompt.slice(i+"[INST]".length, j)}
					</p>
				))
			}
			j += "[INST]".length
			break;
		case -1:
		default:
			j = prompt.length-i
			entries.push((
				<p key={i}>
					{prompt.slice(i, j)}
				</p>
			))
		}
	}


	return (
		<Frame>
			<div class="d-flex flex-column w-100">
				<div class="card text-bg-dark flex-fill overflow-x-auto">
					<div class="card-header">
						Chat
					</div>
					<div class="card-body">
						{entries}
						<div class="w-100 d-flex"  >
							<form onSubmit={submit}>
								<input type="text" class="mb-1 flex-grow-1" ref={input} />
							</form>
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
		<Chat />
	</StrictMode>
);
