import { Component } from 'react';
import classNames from 'classnames';
import { arrayOf, node, oneOfType } from 'prop-types';
import { withStyles } from 'material-ui/styles/index';
import MuiSpeedDial from '@material-ui/lab/SpeedDial';
import SpeedDialIcon from '@material-ui/lab/SpeedDialIcon';
import CloseIcon from 'mdi-react/CloseIcon';
import DotsVerticalIcon from 'mdi-react/DotsVerticalIcon';

const IS_TOUCH = 'ontouchstart' in document.documentElement;

@withStyles(theme => ({
  speedDial: {
    position: 'fixed',
    bottom: theme.spacing.double,
    right: theme.spacing.triple,
  },
}))
/**
 * Render a dynamically expanding set of floating action buttons.
 */
export default class SpeedDial extends Component {
  static propTypes = {
    /**
     * A set of `SpeedDialAction`s which will be rendered upon interaction
     * with the base `SpeedDial` floating action button.
     */
    children: oneOfType([arrayOf(node), node]),
  };

  state = {
    open: false,
  };

  handleClick = () => {
    this.setState({
      open: !this.state.open,
    });
  };

  handleOpen = () => {
    this.setState({
      open: true,
    });
  };

  handleClose = () => {
    this.setState({
      open: false,
    });
  };

  render() {
    const { classes, children, className, ...props } = this.props;
    const { open } = this.state;

    return (
      <MuiSpeedDial
        ariaLabel="speed-dial"
        icon={
          <SpeedDialIcon icon={<DotsVerticalIcon />} openIcon={<CloseIcon />} />
        }
        ButtonProps={{ color: 'secondary' }}
        className={classNames(classes.speedDial, className)}
        onBlur={this.handleClose}
        onClick={this.handleClick}
        onClose={this.handleClose}
        onFocus={!IS_TOUCH && this.handleOpen}
        onMouseEnter={!IS_TOUCH && this.handleOpen}
        onMouseLeave={this.handleClose}
        open={open}
        {...props}>
        {children}
      </MuiSpeedDial>
    );
  }
}
