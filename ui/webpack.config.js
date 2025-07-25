const webpack = require('webpack')
const path = require('path')
const HtmlWebpackPlugin = require('html-webpack-plugin')
const {CleanWebpackPlugin} = require('clean-webpack-plugin')
const CopyWebpackPlugin = require('copy-webpack-plugin')

module.exports = {
  entry: './src/playground.js',
  plugins: [
    new CleanWebpackPlugin({
      // without this, the plugin appears to remove images
      // copied by CopyWebpackPlugin in watch mode
      cleanStaleWebpackAssets: false,
      verbose: true,
    }),
    new HtmlWebpackPlugin({
      template: './src/index.html'
    }),
    new webpack.ProgressPlugin(),
    new CopyWebpackPlugin([
      {from: './src/images', to: 'images'},
    ]),
  ],
  module: {
    rules: [
      {
        test: /\.m?js$/,
        include: path.resolve(__dirname, 'src'),
        use: {
          loader: 'babel-loader',
          options: {
            presets: [
              ['@babel/preset-env', { targets: { "ie": "11" } }]
              // TODO(sr): was this, didn't work
              // ['@babel/preset-env', {
              //   targets: "defaults"
              // }]
            ]
          }
        }
      },
      {
        test: /\.css$/,
        use: [
          'style-loader',
          {
            loader: 'css-loader',
            options: {
              url: false,
            }
          }
        ],
      },
      {
        test: /\.(png|svg|jpg|gif)$/,
        use: [
          'file-loader',
        ],
      },
    ]
  },
  output: {
    filename: '[name].js',
    path: path.resolve(__dirname, '..', 'build', 'ui'),
    publicPath: '/'
  }
}
