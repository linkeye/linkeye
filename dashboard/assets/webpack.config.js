const webpack = require('webpack');
const path = require('path');

module.exports = {
	resolve: {
		extensions: ['.js', '.jsx'],
	},
	entry:  './index',
	output: {
		path:     path.resolve(__dirname, ''),
		filename: 'bundle.js',
	},
	plugins: [
		new webpack.optimize.UglifyJsPlugin({
			comments: false,
			mangle:   false,
			beautify: true,
		}),
		new webpack.DefinePlugin({
			PROD: process.env.NODE_ENV === 'production',
		}),
	],
	module: {
		rules: [
			{
				test:    /\.jsx$/, // regexp for JSX files
				exclude: /node_modules/,
				use:     [ // order: from bottom to top
					{
						loader:  'babel-loader',
						options: {
							plugins: [ // order: from top to bottom
								// 'transform-decorators-legacy', // @withStyles, @withTheme
								'transform-class-properties', // static defaultProps
								'transform-flow-strip-types',
							],
							presets: [ // order: from bottom to top
								'env',
								'react',
								'stage-0',
							],
						},
					},
					// 'eslint-loader', // show errors not only in the editor, but also in the console
				],
			},
			{
				test: /font-awesome\.css$/,
				use:  [
					'style-loader',
					'css-loader',
					path.resolve(__dirname, './fa-only-woff-loader.js'),
				],
			},
			{
				test: /\.woff2?$/, // font-awesome icons
				use:  'url-loader',
			},
		],
	},
};
