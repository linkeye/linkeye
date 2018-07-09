// @flow


type ProvidedMenuProp = {|title: string, icon: string|};
const menuSkeletons: Array<{|id: string, menu: ProvidedMenuProp|}> = [
	{
		id:   'home',
		menu: {
			title: 'Home',
			icon:  'home',
		},
	}, {
		id:   'chain',
		menu: {
			title: 'Chain',
			icon:  'link',
		},
	}, {
		id:   'txpool',
		menu: {
			title: 'TxPool',
			icon:  'credit-card',
		},
	}, {
		id:   'network',
		menu: {
			title: 'Network',
			icon:  'globe',
		},
	}, {
		id:   'system',
		menu: {
			title: 'System',
			icon:  'tachometer',
		},
	}, {
		id:   'logs',
		menu: {
			title: 'Logs',
			icon:  'list',
		},
	},
];
export type MenuProp = {|...ProvidedMenuProp, id: string|};
// The sidebar menu and the main content are rendered based on these elements.
// Using the id is circumstantial in some cases, so it is better to insert it also as a value.
// This way the mistyping is prevented.
export const MENU: Map<string, {...MenuProp}> = new Map(menuSkeletons.map(({id, menu}) => ([id, {id, ...menu}])));

export const DURATION = 200;

export const styles = {
	light: {
		color: 'rgba(255, 255, 255, 0.54)',
	},
}
