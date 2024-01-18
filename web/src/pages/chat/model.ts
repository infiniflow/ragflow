import { Effect, Reducer, Subscription } from 'umi';

export interface chatModelState {
    name: string;
}

export interface chatModelType {
    namespace: 'chatModel';
    state: chatModelState;
    effects: {
        query: Effect;
    };
    reducers: {
        save: Reducer<chatModelState>;
    };
    subscriptions: { setup: Subscription };
}

const Model: chatModelType = {
    namespace: 'chatModel',
    state: {
        name: 'kate',
    },

    effects: {
        *query({ payload }, { call, put }) { },
    },
    reducers: {
        save(state, action) {
            return {
                ...state,
                ...action.payload,
            };
        },
    },
    subscriptions: {
        setup({ dispatch, history }) {
            return history.listen((query) => {
                console.log(query)

            });
        },
    },
};

export default Model;