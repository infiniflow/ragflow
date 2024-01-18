import React from 'react';
import { connect, Dispatch } from 'umi';
import type { chatModelState } from './model'

interface chatProps {
    chatModel: chatModelState;
    dispatch: Dispatch
}

const View: React.FC<chatProps> = ({ chatModel, dispatch }) => {
    const { name } = chatModel;
    return <div>chat:{name} </div>;
};

export default connect(({ chatModel, loading }) => ({ chatModel, loading }))(View);