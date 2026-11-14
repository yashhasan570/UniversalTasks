export function setTitle(title : undefined | string) {
	document.title = (typeof title === 'undefined' || title === '')
		? 'Universal Tasks'
		: `${title} | Universal Tasks`
}
