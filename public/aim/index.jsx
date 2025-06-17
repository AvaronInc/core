import React, {StrictMode, useState, useEffect, useRef, useCallback} from 'react'
import Frame from '../frame'
import {size} from '../util'
import ReactDOM from 'react-dom/client';

function parseTick(input) {
	const output = []
	let i, j = 0
	for (i = 0; i < input.length; i++) {
		let token = ""
		switch(input[i]) {
		case '\\':
			input = input.slice(0, i) + input.slice(i+1)
			continue
		case '`':
			break
		default:
			continue
		}

		output.push(input.slice(j, i))
		return { i, output }
	}

	output.push(input.slice(j, i))
	return { i, output }
}

function parse(input, end) {
	const output = []
	let i, j = 0
	for (i = 0; i < input.length; i++) {
		let token = ""
		switch(input[i]) {
		case '\\':
			input = input.slice(0, i) + input.slice(i+1)
			break;
		case '*':
			if (i+1 < input.length && input[i+1] == '*') {
				token = "**"
			} else {
				token = "*"
			}
			break
		case '_':
			if (i+1 < input.length && input[i+1] == '_') {
				token = "__"
			} else {
				token = "_"
			}
			break
		case '`':
			token = "`"
			break
		}

		if (end !== null && token === end) {
			output.push(input.slice(j, i))
			return { i, output }
		} else if (token === "") {
			continue
		}

		output.push(input.slice(j, i))
		i += token.length
		let result
		switch (token) {
		case "*": case "_":
			result = parse(input.slice(i), token)
			i += result.i+token.length
			j = i
			output.push((
				<em>
					{result.output}
				</em>
			))
			break
		case "**": case "__":
			result = parse(input.slice(i), token)
			i += result.i+token.length
			j = i
			output.push((
				<strong>
					{result.output}
				</strong>
			))
			break
		case "`":
			result = parseTick(input.slice(i), token)
			i += result.i+token.length
			j = i
			output.push((
				<tt>
					{result.output}
				</tt>
			))
			break
		}
	}

	output.push(input.slice(j, i))
	return { i, output }
}

function ordered(line) {
	return (/^[ \t]+[0-9]+\)/).test(line)
}

function unordered(line) {
	return (/^[ \t]*\*[ \t]+/).test(line)
}

function space(line) {
	return (/^\s*$/).test(line)
}


function paragraphs(paragraphs) {
	const base = new Array()
	for (let i = 0; i < paragraphs.length; i++) {
		const lines = paragraphs[i].split("\n").map(line => line + "\n")

		if (lines.every(unordered)) {
			const rows = new Array(lines.length)
			for (let j = 0; j < lines.length; j++) {
				const content = lines[j].slice(lines[j].indexOf('*')+1)
				rows[j] = (
					<li>
						{parse(content, null).output}
					</li>
				)
			}
			base.push((
				<ul>
					{rows}
				</ul>
			))
		} else if (lines.every(ordered)) {
			const rows = new Array(lines.length)
			for (let j = 0; j < lines.length; j++) {
				const content = lines[j].slice(lines[j].indexOf(')')+1)
				rows[j] = (
					<li>
						{parse(content, null).output}
					</li>
				)
			}
			base.push((
				<ol>
					{rows}
				</ol>
			))
		} else {
			const rows = new Array()
			for (let j = 0; j < lines.length; j++) {
				if (lines[j].startsWith("HEALTHY") || space(lines[j])) {
					continue
				}
				rows.push(parse(lines[j], null).output)
			}

			if (rows.length === 0) {
				continue
			}

			base.push((
				<p class="">
					{rows}
				</p>
			))
		}
	}

	return base
}

function markdown(input) {
	const output = new Array()
	const re = /```/g

	let match, n, i, j = 0
	for (n = 0, i = 0; i < input.length; n++) {
		if ((match = (re.exec(input)))) { 
			i = match.index
		} else {
			i = input.length
		}

		if (n % 2) {
			output.push((
				<pre><code>
					{input.slice(j, i)}
				</code></pre>
			))
		} else {
			const p = paragraphs(input.slice(j, i).split("\n\n"))
			output.push(p)
		}
		j = i + "```".length
	}

	return output
}

function style(s) {
	switch (s) {
	case "healthy": return "success";
	case "unhealthy": return "danger";
	default: return "warning";
	}
}

const Chat = () => {
	const [prompt,     setPrompt] = useState("")
	const [entries,   setEntries] = useState({})
	const [textArea, setTextArea] = useState(false)
	const input                   = useRef("")
	const xhr                     = useRef(null)

	const fetchEntries = useCallback(() => {
		fetch("/api/health")
			.then(r => r.json())
			.then((r) => (console.log(r), r))
			.then(setEntries)
	}, [setEntries])

	const submit = useCallback((e) => {
		e.preventDefault()
		if (!input.current || !input.current.value) {
			return
		}

		if (xhr.current) {
			xhr.current.abort()
			xhr.current = null
		}

		xhr.current = new XMLHttpRequest();
		xhr.current.open("POST", "/api/completions", true);

		let i = 0, j = 0
		xhr.current.onreadystatechange = function(e) {
			switch (xhr.current.readyState) {
			case 3: break;
			default:
				return
			}

			for (; i < xhr.current.responseText.length; i += j+1) {
				const rest = xhr.current.responseText.slice(i)

				j = rest.indexOf('\n')
				if (j == -1) {
					break
				} else if (j == 0) {
					continue
				}

				const token = JSON.parse(rest.slice("data: ".length, j))
				setPrompt(chat => chat + token.content)
			}
		};

		setPrompt(p => {
			p = p+"[INST]"+input.current.value+"[/INST]"
			xhr.current.send(JSON.stringify({
				prompt: p,
				model: "mixtral.gguf",
				stream: true,
			}));
			input.current.value = ""
			return p
		})

	}, [setPrompt])

	const focusLog = useCallback((ts) => {
		if (xhr.current) {
			xhr.current.abort()
			xhr.current = null
		}

		xhr.current = new XMLHttpRequest();
		xhr.current.open("GET", "/api/health/" + ts, true);

		xhr.current.onreadystatechange = function(e) {
			switch (e.target.readyState){
			case 4: break
			default:
				if (e.target.responseText.length <= 0) {
					return
				}
			}
			setPrompt(e.target.responseText)
		};

		xhr.current.send();
	}, [setPrompt])

	useEffect(fetchEntries, [])

	let i, j;
	const messages = new Array()

	for (i = 0; i < prompt.length; i += j) {
		j = prompt.slice(i).indexOf("[INST]")

		switch (j) {
		case 0:
			j = prompt.slice(i).indexOf("[/INST]")
			if (j == -1) {
				throw "bad prompt"
			} else if (j > "[INST]".length) {
				const message = markdown(prompt.slice(i+"[INST]".length, i+j))
				messages.push((
					<div class="ms-auto bg-secondary p-3 pb-1 rounded mb-1">
						<p key={i}>
							{message ? message : "..."}
						</p>
					</div>
				))
			}
			j += "[/INST]".length
			break;
		case -1:
			j = prompt.length
		default:
			if (i == 0) {
				continue
			}
			if (space(prompt.slice(i, i+j))) {
				break
			}
			const message = markdown(prompt.slice(i, i+j))
			messages.push((
				<div key={i} class="me-auto bg-primary p-2 pb-1 rounded mb-1">
					{message ? message : "..."}
				</div>
			))
		}
	}

	i = size(entries)
	const logs = new Array(i)
	for (entry in entries) {
		logs[--i] = ((
			<tr onClick={focusLog.bind(null, entry)} id={entry} class="pb-3">
				<td><tt>
					{new Date(entry*1000).toString()}
				</tt></td>
				<td class={"m-1 py-1 px-2 rounded text-center bg-"+style(entries[entry])}>
					{entries[entry]}
				</td>
			</tr>
		))
	}

	return (
		<Frame>
			<div class="d-flex flex-column w-100">

				<div class="card text-bg-dark flex-fill overflow-x-auto mb-2">
					<div class="card-header">
						Chat
					</div>
					<div class="card-body d-flex flex-column">
						{messages}
						<div class="w-100 d-flex"  >
							<form class="w-100 d-flex flex-row" onSubmit={submit}>
								<div class="input-group">  
									{textArea ? (
										<textarea class="form-control" ref={input}/>
									) : (
										<input type="text" class="form-control" ref={input}/>
									)}
									<input
										value={textArea ? "▭" : "▯" }
										class="btn btn-outline-secondary"
										type="button"
										onClick={setTextArea.bind(null, !textArea)}
									/>
									<input
										value="↑"
										onClick={submit}
										class="btn btn-outline-primary"
										type="button"
									/>
								</div>
							</form>
						</div>
					</div>
				</div>
				<div class="card text-bg-dark flex-fill overflow-x-auto">
					<div class="card-header">
						Health Checks
					</div>
					<div class="card-body d-flex flex-column">
						<table>
							{logs}
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
		<Chat />
	</StrictMode>
);
