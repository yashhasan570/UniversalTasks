import type {Prefixes} from './types'

const UNIVERSALTASKS_PREFIXES: Prefixes = {
	label: '*',
	project: '+',
	priority: '!',
	assignee: '@',
}

const TODOIST_PREFIXES: Prefixes = {
	label: '@',
	project: '#',
	priority: '!',
	assignee: '+',
}

export enum PrefixMode {
	Disabled = 'disabled',
	Default = 'universaltasks',
	Todoist = 'todoist',
}

export const PREFIXES = {
	[PrefixMode.Disabled]: undefined,
	[PrefixMode.Default]: UNIVERSALTASKS_PREFIXES,
	[PrefixMode.Todoist]: TODOIST_PREFIXES,
}
