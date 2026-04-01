package buyer

// GetState returns buyer state for a valid session.
func (m *Manager) GetState(sessionID string) (State, error) {
	acc, err := m.getAccountBySession(sessionID)
	if err != nil {
		return State{}, err
	}
	return acc.State, nil
}
