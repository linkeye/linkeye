// @flow


import React, {Component} from 'react';

import withStyles from 'material-ui/styles/withStyles';

import {MENU} from '../common';
import Footer from './Footer';
import type {Content} from '../types/content';

// styles contains the constant styles of the component.
const styles = {
	wrapper: {
		display:       'flex',
		flexDirection: 'column',
		width:         '100%',
	},
	content: {
		flex:     1,
		overflow: 'auto',
	},
};

// themeStyles returns the styles generated from the theme for the component.
const themeStyles = theme => ({
	content: {
		backgroundColor: theme.palette.background.default,
		padding:         theme.spacing.unit * 3,
	},
});

export type Props = {
	classes: Object,
	active: string,
	content: Content,
	shouldUpdate: Object,
};

// Main renders the chosen content.
class Main extends Component<Props> {
	render() {
		const {
			classes, active, content, shouldUpdate,
		} = this.props;

		let children = null;
		switch (active) {
		case MENU.get('home').id:
		case MENU.get('chain').id:
		case MENU.get('txpool').id:
		case MENU.get('network').id:
		case MENU.get('system').id:
			children = <div>Work in progress.</div>;
			break;
		case MENU.get('logs').id:
			children = <div>{content.logs.log.map((log, index) => <div key={index}>{log}</div>)}</div>;
		}

		return (
			<div style={styles.wrapper}>
				<div className={classes.content} style={styles.content}>{children}</div>
				<Footer
					general={content.general}
					system={content.system}
					shouldUpdate={shouldUpdate}
				/>
			</div>
		);
	}
}

export default withStyles(themeStyles)(Main);
