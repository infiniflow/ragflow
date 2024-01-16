import React, { useReducer } from 'react'
const CHANGE_LOCALE = 'CHANGE_LOCALE'

const mainContext = React.createContext()

const reducer = (state, action) => {
    switch (action.type) {
        case CHANGE_LOCALE:
            return { ...state, locale: action.locale || 'zh' }
        default:
            return state
    }
}

const ContextProvider = (props) => {
    const [state, dispatch] = useReducer(reducer, {
        locale: 'zh'
    })
    return (
        <mainContext.Provider value={{ state, dispatch }}>
            {props.children}
        </mainContext.Provider>
    )
}

export { reducer, mainContext, ContextProvider }

