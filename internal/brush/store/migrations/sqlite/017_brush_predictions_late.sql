UPDATE brush_predictions SET outcome = 'lost', actual = 0 WHERE outcome = 'resolved' AND actual_seconds > horizon_seconds + 300
