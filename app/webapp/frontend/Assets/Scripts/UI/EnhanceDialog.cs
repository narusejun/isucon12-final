using System;
using System.Collections;
using System.Collections.Generic;
using Data;
using UnityEngine;
using UnityEngine.UI;

public class EnhanceDialog : MonoBehaviour
{
    [SerializeField] private Button _closeButton;

    private UserCard _card;

    public void SetCard(UserCard card, Action onClose)
    {
        _card = card;
        
        _closeButton.onClick.AddListener(() => onClose());
    }
}
