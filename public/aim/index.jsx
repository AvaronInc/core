import React, {StrictMode, useState, useEffect, useRef, useCallback} from 'react'
import Frame from '../frame'
import {size} from '../util'
import ReactDOM from 'react-dom/client';

const test = `
[INST]show me bold & italic in markdown[/INST] Sure! Here is how you can make text bold and italic in Markdown:

* To make text bold in Markdown, you can surround the text with two asterisks (\`**\`) or two underscores (\`__\`) on each side. For example:

\`**This is bold text.**\`

or

\`__This is bold text.__\`

* To make text italic in Markdown, you can surround the text with one asterisk (\`*\`) or one underscore (\`_\`) on each side. For example:

This is escaped \\*asterisk\\*.

\`*This is italic text.*\`

or

\`\`\`
foo bar ipsum code
\`\`\`


\`_This is italic text._\`

* To make text both bold and italic in Markdown, you can surround the text with three asterisks (\`***\`) or three underscores (\`___\`) on each side. For example:

\`***This is bold and italic text.***\`

or

\`___This is bold and italic text.___\`

I hope this helps! Let me know if you have any other questions.

**This is bold text.**
***This is bold and italic text.***

`

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
		const lines = paragraphs[i].split("\n")
		const rows = new Array(lines.length)

		if (lines.every(unordered)) {
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
			for (let j = 0; j < lines.length; j++) {
				rows[j] = parse(lines[j], null).output
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
	const re = /\`\`\`/gm

	let match, n, i, j = 0
	for (n = 0, i = 0; i < input.length; n++) {
		if ((match = (re.exec(input)))) { 
			i = match.index
		} else {
			i = input.length
		}

		if (n % 2) {
			output.push((
				<code>
					{input.slice(j, i)}
				</code>
			))
		} else {
			const p = paragraphs(input.slice(j, i).split("\n\n"))
			output.push(p)
		}
		j = i + "```".length
	}

	return output
}


const Chat = () => {
	const [prompt,     setPrompt] = useState(test)
	const [logs,         setLogs] = useState([])
	const [textArea, setTextAreaInner] = useState(false)
	const input               = useRef("")
	const xhr                 = useRef(null)
	const setTextArea = (v) => {
		setTextAreaInner(v)
	}

	const fetchLogs = useCallback(() => {
		fetch("/api/logs")
			.then(r => r.json())
			.then(setLogs)
	}, [setLogs])

	const submit = useCallback((e) => {
		e.preventDefault()
		if (!input.current || !input.current.value) {
			return
		}

		if (xhr.current != null) {
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
			return p
		})

	}, [setPrompt])

	useEffect(fetchLogs, [])

	let i, j;
	const entries = new Array()

	for (i = 0; i < prompt.length; i += j) {
		j = prompt.slice(i).indexOf("[INST]")

		console.log("J", j)
		switch (j) {
		case 0:
			j = prompt.slice(i).indexOf("[/INST]")
			console.log("pj", j)
			if (j == -1) {
				throw "bad prompt"
			} else if (j > "[INST]".length) {
				entries.push((
					<div class="ms-auto bg-success p-3 pb-1 rounded mb-1">
						<p class=" text-end" key={i}>
							{prompt.slice(i+"[INST]".length, i+j)}
						</p>
					</div>
				))
			}
			j += "[/INST]".length
			break;
		case -1:
			j = prompt.length
		default:
			if (space(prompt.slice(i, i+j))) {
				break
			}
			const message = markdown(prompt.slice(i, i+j))
			console.log("i, i+j", prompt.slice(i, i+j))
			console.log("i+j", prompt.slice(i+j))
			entries.push((
				<div key={i} class="me-auto bg-primary p-2 pb-1 rounded mb-1">
					{message ? message : "..."}
				</div>
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
					<div class="card-body d-flex flex-column">
						{entries}
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
