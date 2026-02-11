import type { Meta, StoryObj } from '@storybook/react';
import StatusIndicator from './StatusIndicator';

const meta: Meta<typeof StatusIndicator> = {
  title: 'Components/StatusIndicator',
  component: StatusIndicator,
  tags: ['autodocs'],
  argTypes: {
    status: {
      control: 'select',
      options: ['healthy', 'warning', 'critical'],
      description: '状态类型',
    },
    size: {
      control: 'select',
      options: ['small', 'default', 'large'],
      description: '指示器大小',
    },
    showText: {
      control: 'boolean',
      description: '是否显示文字',
    },
  },
};

export default meta;

type Story = StoryObj<typeof StatusIndicator>;

export const Healthy: Story = {
  args: {
    status: 'healthy',
    showText: true,
  },
};

export const Warning: Story = {
  args: {
    status: 'warning',
    showText: true,
  },
};

export const Critical: Story = {
  args: {
    status: 'critical',
    showText: true,
  },
};

export const SmallSize: Story = {
  args: {
    status: 'healthy',
    size: 'small',
    showText: false,
  },
};

export const LargeSize: Story = {
  args: {
    status: 'healthy',
    size: 'large',
    showText: true,
  },
};
