import { Effect, ImmerReducer, Reducer, Subscription } from 'umi';

export interface IndexModelState {
    name: string;
}

export interface IndexModelType {
    namespace: 'index';
    state: IndexModelState;
    effects: {
        query: Effect;
    };
    reducers: {
        save: Reducer<IndexModelState>;
        // 启用 immer 之后
        // save: ImmerReducer<IndexModelState>;
    };
    subscriptions: { setup: Subscription };
}

const IndexModel: IndexModelType = {
    namespace: 'index',
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
        // 启用 immer 之后
        // save(state, action) {
        //   state.name = action.payload;
        // },
    },
    subscriptions: {
        setup({ dispatch, history }) {
            return history.listen((query) => {
                console.log(query)

            });
        },
    },
};

export default IndexModel;